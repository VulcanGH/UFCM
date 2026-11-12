// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/acquisition"
	"github.com/autobrr/autobrr/internal/ufc/automation"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/pkg/errors"
)

// AutomationSettings reads the existing Foundation switch.
func (r *Repository) AutomationSettings(ctx context.Context) (*automation.Settings, error) {
	s, err := r.LoadApplicationSettings(ctx)
	return &automation.Settings{Revision: s.Revision, Enabled: s.AutomaticAcquisitionEnabled}, err
}

// SaveAutomationSettings never starts acquisition as part of the HTTP request.
func (r *Repository) SaveAutomationSettings(ctx context.Context, input automation.Settings) (*automation.Settings, error) {
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		if input.Enabled {
			var raw string
			if err := tx.QueryRowContext(ctx, "SELECT settings_json FROM ufc_processing_settings WHERE id=1").Scan(&raw); err != nil {
				return err
			}
			var target struct {
				Client int `json:"sab_client_id"`
			}
			if err := json.Unmarshal([]byte(raw), &target); err != nil {
				return err
			}
			if target.Client < 1 {
				return errors.Wrap(catalogue.ErrInvalidEdit, "select a SAB client in Processing first")
			}
		}
		result, err := tx.ExecContext(ctx, "UPDATE ufc_application_settings SET automatic_acquisition_enabled=$1,revision=revision+1 WHERE id=1 AND revision=$2", input.Enabled, input.Revision)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return catalogue.ErrRevisionConflict
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.AutomationSettings(ctx)
}

func confirmedComponents(ctx context.Context, tx queryer, event string) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT f.component FROM ufc_library_files f JOIN ufc_library_roots r ON r.id=f.root_id WHERE f.event_id=$1 AND f.present=TRUE AND f.state='confirmed' AND r.enabled=TRUE AND r.state='available'`, event)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]bool{}
	for rows.Next() {
		var component string
		if err := rows.Scan(&component); err != nil {
			return nil, err
		}
		result[component] = true
	}
	return result, rows.Err()
}

func guardAutomaticAcquisition(ctx context.Context, tx *sql.Tx, driver string, j acquisition.Job) error {
	lock := ""
	if driver == "postgres" {
		lock = " FOR SHARE"
	}
	var enabled bool
	if err := tx.QueryRowContext(ctx, "SELECT automatic_acquisition_enabled FROM ufc_application_settings WHERE id=1"+lock).Scan(&enabled); err != nil {
		return err
	}
	if !enabled {
		return errors.Wrap(catalogue.ErrInvalidEdit, "automatic acquisition is disabled")
	}
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM ufc_automation_events a JOIN ufc_event_workspace w ON w.event_id=a.event_id WHERE a.event_id=$1 AND a.archived_at=''`, j.EventID).Scan(&active); err != nil {
		return err
	}
	event, err := automationEvent(ctx, tx, j.EventID)
	if err != nil {
		return err
	}
	wanted := false
	for _, key := range event.Components.Wanted {
		wanted = wanted || key == j.Component
	}
	if active != 1 || !automation.Active(event, time.Now()) || !wanted || len(event.Review) > 0 {
		return errors.Wrap(catalogue.ErrInvalidEdit, "event is no longer eligible for automation")
	}
	var raw string
	if err := tx.QueryRowContext(ctx, "SELECT settings_json FROM ufc_processing_settings WHERE id=1"+lock).Scan(&raw); err != nil {
		return err
	}
	var settings struct {
		ClientID int32  `json:"sab_client_id"`
		Category string `json:"sab_category"`
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return err
	}
	if settings.ClientID != j.ClientID || settings.Category != j.Category {
		return errors.Wrap(catalogue.ErrInvalidEdit, "saved SAB target changed")
	}
	return nil
}

func loadWorkspaceState(ctx context.Context, q queryer, event *catalogue.Event) error {
	var archived string
	err := q.QueryRowContext(ctx, "SELECT archived_at FROM ufc_automation_events WHERE event_id=$1", event.ID).Scan(&archived)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	past := archived != "" || automation.ClosedWindow(*event, time.Now())
	if !past {
		if event.Components == nil {
			if err := loadComponentPreferences(ctx, q, event); err != nil {
				return err
			}
		}
		coverage, err := confirmedComponents(ctx, q, event.ID)
		if err != nil {
			return err
		}
		var scanning bool
		if err := q.QueryRowContext(ctx, "SELECT running FROM ufc_library_settings WHERE id=1").Scan(&scanning); err != nil {
			return err
		}
		complete := !scanning
		for _, key := range event.Components.Wanted {
			complete = complete && coverage[key]
		}
		past = complete
	}
	event.WorkspacePast = &past
	return nil
}

func automationEvent(ctx context.Context, tx *sql.Tx, id string) (catalogue.Event, error) {
	e, err := decodeEvent(tx.QueryRowContext(ctx, "SELECT "+eventColumns+" FROM ufc_events WHERE id=$1", id))
	if err != nil {
		return e, err
	}
	if err = loadAliases(ctx, tx, &e); err != nil {
		return e, err
	}
	err = loadComponentPreferences(ctx, tx, &e)
	return e, err
}

// AutomationEvents admits only upcoming workspace events and closes completed/expired memberships.
func (r *Repository) AutomationEvents(ctx context.Context, now time.Time) ([]catalogue.Event, error) {
	result := []catalogue.Event{}
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, `SELECT id FROM ufc_events WHERE id IN (SELECT event_id FROM ufc_event_workspace) AND status='scheduled' AND event_date >= $1 AND id NOT IN (SELECT event_id FROM ufc_automation_events) ORDER BY event_date,id LIMIT 100`, now.UTC().Format("2006-01-02"))
		if err != nil {
			return err
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, id := range ids {
			e, err := automationEvent(ctx, tx, id)
			if err != nil {
				return err
			}
			if automation.CanAdmit(e, now) {
				if _, err := tx.ExecContext(ctx, "INSERT INTO ufc_automation_events(event_id,admitted_at) VALUES($1,$2) ON CONFLICT DO NOTHING", id, now.UTC().Format(time.RFC3339Nano)); err != nil {
					return err
				}
			}
		}
		rows, err = tx.QueryContext(ctx, "SELECT event_id FROM ufc_automation_events WHERE archived_at='' ORDER BY event_id")
		if err != nil {
			return err
		}
		ids = nil
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, id := range ids {
			e, err := automationEvent(ctx, tx, id)
			if err != nil {
				return err
			}
			coverage, err := confirmedComponents(ctx, tx, id)
			if err != nil {
				return err
			}
			var scanning bool
			if err := tx.QueryRowContext(ctx, "SELECT running FROM ufc_library_settings WHERE id=1").Scan(&scanning); err != nil {
				return err
			}
			complete := !scanning
			for _, key := range e.Components.Wanted {
				complete = complete && coverage[key]
			}
			reason := ""
			if complete {
				reason = "wanted_components_complete"
			} else if automation.ClosedWindow(e, now) {
				reason = "window_closed"
			}
			var membership int
			if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ufc_event_workspace WHERE event_id=$1", id).Scan(&membership); err != nil {
				return err
			}
			if membership == 0 && reason == "" {
				continue
			}
			if reason != "" {
				if _, err := tx.ExecContext(ctx, "UPDATE ufc_automation_events SET archived_at=$1,reason=$2 WHERE event_id=$3 AND archived_at=''", now.UTC().Format(time.RFC3339Nano), reason, id); err != nil {
					return err
				}
			} else {
				result = append(result, e)
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE ufc_automation_inbox SET state='rejected',message='Event acquisition window closed',item_json='{}' WHERE state='pending' AND event_id IN (SELECT event_id FROM ufc_automation_events WHERE archived_at<>'')`)
		return err
	})
	return result, err
}

// EnqueueAutomationItem journals a candidate before the feed marks its page seen.
func (r *Repository) EnqueueAutomationItem(ctx context.Context, item automation.Item) error {
	raw, err := json.Marshal(struct {
		URL, Reference, Title string
		Size                  uint64
		PublishedAt           *time.Time
	}{item.Release.URL, item.Release.Reference, item.Release.Title, item.Release.Size, item.Release.PublishedAt})
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = r.db.ExecContext(ctx, `INSERT INTO ufc_automation_inbox(id,feed_id,config_key,event_id,component,item_json,created_at,next_due) VALUES($1,$2,$3,$4,$5,$6,$7,$7) ON CONFLICT DO NOTHING`, item.ID, item.FeedID, item.ConfigKey, item.EventID, item.Component, string(raw), now)
	return err
}

// AutomationItems reads bounded work after restart without replaying completed decisions.
func (r *Repository) AutomationItems(ctx context.Context, now time.Time) ([]automation.Item, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id,feed_id,config_key,event_id,component,item_json,attempts FROM ufc_automation_inbox WHERE state='pending' AND next_due<=$1 ORDER BY created_at,id LIMIT 100", now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []automation.Item{}
	for rows.Next() {
		var item automation.Item
		var raw string
		if err := rows.Scan(&item.ID, &item.FeedID, &item.ConfigKey, &item.EventID, &item.Component, &raw, &item.Attempts); err != nil {
			return nil, err
		}
		var release struct {
			URL, Reference, Title string
			Size                  uint64
			PublishedAt           *time.Time
		}
		if err := json.Unmarshal([]byte(raw), &release); err != nil {
			return nil, err
		}
		item.Release.URL = release.URL
		item.Release.Reference = release.Reference
		item.Release.Title = release.Title
		item.Release.Size = release.Size
		item.Release.PublishedAt = release.PublishedAt
		result = append(result, item)
	}
	return result, rows.Err()
}

// FinishAutomationItem records decisions or bounded infrastructure backoff without replaying submissions.
func (r *Repository) FinishAutomationItem(ctx context.Context, id, state, message string, next time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ufc_automation_inbox SET state=$1,message=$2,attempts=attempts+1,next_due=$3,item_json=CASE WHEN $1='pending' THEN item_json ELSE '{}' END WHERE id=$4 AND state='pending'`, state, message, next.UTC().Format(time.RFC3339Nano), id)
	return err
}

func queueUFCNotice(ctx context.Context, tx *sql.Tx, id, event, title, message string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := tx.ExecContext(ctx, `INSERT INTO ufc_notification_outbox(id,event,title,message,next_due,created_at) VALUES($1,$2,$3,$4,$5,$5) ON CONFLICT DO NOTHING`, id, event, title, message, now)
	return err
}

func queueProcessingNotice(ctx context.Context, tx *sql.Tx, id, event, message string) error {
	var title string
	err := tx.QueryRowContext(ctx, `SELECT COALESCE(e.name,j.event_id) FROM ufc_processing_jobs j LEFT JOIN ufc_events e ON e.id=j.event_id WHERE j.id=$1`, id).Scan(&title)
	if err != nil {
		return err
	}
	return queueUFCNotice(ctx, tx, event+":"+id+":"+message, event, title, message)
}

func archiveCompletedEvent(ctx context.Context, tx *sql.Tx, id string) error {
	var active int
	if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ufc_automation_events WHERE event_id=$1 AND archived_at=''", id).Scan(&active); err != nil {
		return err
	}
	if active == 0 {
		return nil
	}
	event, err := automationEvent(ctx, tx, id)
	if err != nil {
		return err
	}
	coverage, err := confirmedComponents(ctx, tx, id)
	if err != nil {
		return err
	}
	for _, key := range event.Components.Wanted {
		if !coverage[key] {
			return nil
		}
	}
	_, err = tx.ExecContext(ctx, "UPDATE ufc_automation_events SET archived_at=$1,reason='wanted_components_complete' WHERE event_id=$2 AND archived_at=''", time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}

// AutomationNotices reads notification work separately from acquisition work.
func (r *Repository) AutomationNotices(ctx context.Context, now time.Time) ([]automation.Notice, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id,event,title,message,attempts FROM ufc_notification_outbox WHERE delivered=FALSE AND next_due<=$1 ORDER BY created_at,id LIMIT 20", now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []automation.Notice{}
	for rows.Next() {
		var n automation.Notice
		if err := rows.Scan(&n.ID, &n.Event, &n.Title, &n.Message, &n.Attempts); err != nil {
			return nil, err
		}
		result = append(result, n)
	}
	return result, rows.Err()
}

// FinishAutomationNotice retries delivery only, never downloading or copying again.
func (r *Repository) FinishAutomationNotice(ctx context.Context, id string, delivered bool, next time.Time) error {
	_, err := r.db.ExecContext(ctx, "UPDATE ufc_notification_outbox SET delivered=$1,attempts=attempts+1,next_due=$2 WHERE id=$3 AND delivered=FALSE", delivered, next.UTC().Format(time.RFC3339Nano), id)
	return err
}
