// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/pkg/errors"
)

func readPreferences(ctx context.Context, q queryer) (*catalogue.Settings, error) {
	s := &catalogue.Settings{EventCounts: map[string]int{"numbered": 0, "fight_night": 0}}
	var raw string
	var qualityRaw string
	var termsRaw string
	if err := q.QueryRowContext(ctx, "SELECT main_card_terms_json FROM ufc_catalogue_settings WHERE id=1").Scan(&termsRaw); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(termsRaw), &s.MainCardTerms); err != nil {
		return nil, err
	}
	if err := catalogue.ValidateMainCardTerms(s.MainCardTerms); err != nil {
		return nil, err
	}
	if err := q.QueryRowContext(ctx, "SELECT resolutions_json FROM ufc_catalogue_settings WHERE id=1").Scan(&qualityRaw); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(qualityRaw), &s.Resolutions); err != nil {
		return nil, err
	}
	if err := catalogue.ValidateResolutions(s.Resolutions); err != nil {
		return nil, err
	}
	if err := q.QueryRowContext(ctx, "SELECT revision, component_templates_json, periodic_sync_enabled, sync_interval_seconds, next_due FROM ufc_catalogue_settings WHERE id=1").Scan(&s.Revision, &raw, &s.PeriodicSyncEnabled, &s.SyncIntervalSeconds, &s.NextDue); err != nil {
		return nil, errors.Wrap(err, "read component defaults")
	}
	if err := json.Unmarshal([]byte(raw), &s.Templates); err != nil {
		return nil, errors.Wrap(err, "decode component defaults")
	}
	if s.Revision < 1 {
		return nil, errors.New("invalid component settings revision")
	}
	if err := catalogue.ValidateTemplates(s.Templates); err != nil {
		return nil, err
	}
	if s.SyncIntervalSeconds < 3600 || s.SyncIntervalSeconds > 604800 {
		return nil, errors.New("invalid metadata refresh interval")
	}
	rows, err := q.QueryContext(ctx, "SELECT event_type, COUNT(*) FROM ufc_events WHERE manual_corrections_json NOT LIKE '%\"deleted\":true%' GROUP BY event_type")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var eventType string
		var count int
		if err := rows.Scan(&eventType, &count); err != nil {
			return nil, err
		}
		s.EventCounts[eventType] = count
	}
	return s, rows.Err()
}

// CataloguePreferences reads templates and counts from one local snapshot.
func (r *Repository) CataloguePreferences(ctx context.Context) (*catalogue.Settings, error) {
	var result *catalogue.Settings
	err := r.readTransaction(ctx, func(tx *sql.Tx) error {
		var err error
		result, err = readPreferences(ctx, tx)
		return err
	})
	return result, err
}

// SaveCataloguePreferences preserves explicit event overrides and refresh settings.
func (r *Repository) SaveCataloguePreferences(ctx context.Context, edit catalogue.SettingsEdit) (*catalogue.Settings, error) {
	if edit.Revision < 1 {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "settings revision is required")
	}
	if edit.SyncIntervalSeconds != nil && (*edit.SyncIntervalSeconds < 3600 || *edit.SyncIntervalSeconds > 604800) {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "metadata refresh interval must be between one hour and seven days")
	}
	if err := catalogue.ValidateTemplates(edit.Templates); err != nil {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "%s", err.Error())
	}
	raw, err := json.Marshal(edit.Templates)
	if err != nil {
		return nil, err
	}
	var result *catalogue.Settings
	err = r.transaction(ctx, func(tx *sql.Tx) error {
		old, err := readPreferences(ctx, tx)
		if err != nil {
			return err
		}
		if old.Revision != edit.Revision {
			return catalogue.ErrRevisionConflict
		}
		terms := old.MainCardTerms
		if len(edit.MainCardTerms) > 0 {
			if err := json.Unmarshal(edit.MainCardTerms, &terms); err != nil {
				return errors.Wrap(catalogue.ErrInvalidEdit, "invalid exclusion terms")
			}
		}
		if err := catalogue.ValidateMainCardTerms(terms); err != nil {
			return err
		}
		termsRaw, err := json.Marshal(terms)
		if err != nil {
			return err
		}
		resolutions := old.Resolutions
		if edit.Resolutions != nil {
			resolutions = *edit.Resolutions
		}
		if err := catalogue.ValidateResolutions(resolutions); err != nil {
			return err
		}
		qualityRaw, err := json.Marshal(resolutions)
		if err != nil {
			return err
		}
		enabled, interval := old.PeriodicSyncEnabled, old.SyncIntervalSeconds
		if edit.PeriodicSyncEnabled != nil {
			enabled = *edit.PeriodicSyncEnabled
		}
		if edit.SyncIntervalSeconds != nil {
			interval = *edit.SyncIntervalSeconds
		}
		nextDue := old.NextDue
		if !enabled {
			nextDue = nil
		} else if !old.PeriodicSyncEnabled || interval != old.SyncIntervalSeconds || nextDue == nil {
			next := time.Now().UTC().Add(time.Duration(interval) * time.Second).Format(time.RFC3339Nano)
			nextDue = &next
		}
		updated, err := tx.ExecContext(ctx, "UPDATE ufc_catalogue_settings SET component_templates_json=$1, periodic_sync_enabled=$2, sync_interval_seconds=$3, next_due=$4, resolutions_json=$6, main_card_terms_json=$7, revision=revision+1 WHERE id=1 AND revision=$5", string(raw), enabled, interval, nextDue, edit.Revision, string(qualityRaw), string(termsRaw))
		if err != nil {
			return err
		}
		count, err := updated.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return catalogue.ErrRevisionConflict
		}
		result, err = readPreferences(ctx, tx)
		return err
	})
	if err != nil && (strings.Contains(err.Error(), "database is locked") || strings.Contains(err.Error(), "SQLITE_BUSY")) {
		return nil, catalogue.ErrRevisionConflict
	}
	return result, err
}

func loadComponentPreferences(ctx context.Context, q queryer, e *catalogue.Event) error {
	var raw string
	if err := q.QueryRowContext(ctx, "SELECT component_templates_json FROM ufc_catalogue_settings WHERE id=1").Scan(&raw); err != nil {
		return err
	}
	var templates catalogue.Templates
	if err := json.Unmarshal([]byte(raw), &templates); err != nil {
		return err
	}
	if err := catalogue.ValidateTemplates(templates); err != nil {
		return err
	}
	p := &catalogue.ComponentPreferences{Defaults: append([]string{}, templates[e.Values.Type]...), Overrides: map[string]bool{}}
	rows, err := q.QueryContext(ctx, "SELECT component_key, enabled FROM ufc_event_component_overrides WHERE event_id=$1", e.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var enabled bool
		if err := rows.Scan(&key, &enabled); err != nil {
			return err
		}
		p.Overrides[key] = enabled
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := catalogue.ValidateOverrides(p.Overrides); err != nil {
		return err
	}
	p.Wanted = catalogue.Components(e.Values.Type, templates, p.Overrides)
	e.Components = p
	if err := rows.Close(); err != nil {
		return err
	}
	return loadQualityPreferences(ctx, q, e)
}
