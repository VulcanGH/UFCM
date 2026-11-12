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

const eventColumns = "id, revision, event_type, name, event_date, headliners_json, status, source_snapshot_json, manual_corrections_json, review_json"

type scanner interface{ Scan(...any) error }
type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func decodeEvent(row scanner) (catalogue.Event, error) {
	var e catalogue.Event
	var headliners, source, corrections, review string
	var date sql.NullString
	if err := row.Scan(&e.ID, &e.Revision, &e.Values.Type, &e.Values.Name, &date, &headliners, &e.Values.Status, &source, &corrections, &review); err != nil {
		return e, err
	}
	if date.Valid {
		e.Values.Date = &date.String
	}
	for _, field := range []struct {
		raw  string
		dest any
	}{{headliners, &e.Values.Headliners}, {source, &e.Source}, {corrections, &e.Corrections}, {review, &e.Review}} {
		if err := json.Unmarshal([]byte(field.raw), field.dest); err != nil {
			return e, errors.Wrap(err, "invalid persisted catalogue field")
		}
	}
	return e, nil
}

func loadAliases(ctx context.Context, q queryer, e *catalogue.Event) error {
	rows, err := q.QueryContext(ctx, "SELECT namespace, value, is_current, verified, manual FROM ufc_event_aliases WHERE event_id=$1 ORDER BY namespace, normalized_value, is_current", e.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var a catalogue.Alias
		if err := rows.Scan(&a.Namespace, &a.Value, &a.Current, &a.Verified, &a.Manual); err != nil {
			return err
		}
		e.Aliases = append(e.Aliases, a)
	}
	return rows.Err()
}

// LoadCatalogueEvent reads a local event without contacting its source.
func (r *Repository) LoadCatalogueEvent(ctx context.Context, id string) (catalogue.Event, error) {
	var e catalogue.Event
	err := r.readTransaction(ctx, func(tx *sql.Tx) error {
		var err error
		e, err = decodeEvent(tx.QueryRowContext(ctx, "SELECT "+eventColumns+" FROM ufc_events WHERE id=$1", id))
		if errors.Is(err, sql.ErrNoRows) {
			return catalogue.ErrEventNotFound
		}
		if err != nil {
			return errors.Wrap(err, "read catalogue event")
		}
		if e.Corrections.Deleted {
			return catalogue.ErrEventNotFound
		}
		if err := loadAliases(ctx, tx, &e); err != nil {
			return errors.Wrap(err, "read catalogue aliases")
		}
		return errors.Wrap(loadComponentPreferences(ctx, tx, &e), "read component preferences")
	})
	return e, err
}

// EditCatalogueEvent atomically saves protected fields at the supplied revision.
func (r *Repository) EditCatalogueEvent(ctx context.Context, id string, edit catalogue.EventEdit, actor string) (catalogue.Event, error) {
	var event catalogue.Event
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		old, err := decodeEvent(tx.QueryRowContext(ctx, "SELECT "+eventColumns+" FROM ufc_events WHERE id=$1", id))
		if errors.Is(err, sql.ErrNoRows) {
			return catalogue.ErrEventNotFound
		}
		if err != nil {
			return err
		}
		if err := loadAliases(ctx, tx, &old); err != nil {
			return err
		}
		event, err = catalogue.ApplyEdit(old, edit, actor, time.Now())
		if err != nil {
			return err
		}
		if err := checkAliasOwners(ctx, tx, event); err != nil {
			return err
		}
		if err := saveEvent(ctx, tx, event, edit.Revision); err != nil {
			return err
		}
		if q := edit.Quality; q != nil {
			if q.Mode == "inherit" {
				_, err = tx.ExecContext(ctx, "DELETE FROM ufc_event_quality_overrides WHERE event_id=$1", id)
			} else {
				raw, marshalErr := json.Marshal(q.Resolutions)
				if marshalErr != nil {
					return marshalErr
				}
				_, err = tx.ExecContext(ctx, "INSERT INTO ufc_event_quality_overrides(event_id,resolutions_json) VALUES($1,$2) ON CONFLICT(event_id) DO UPDATE SET resolutions_json=excluded.resolutions_json", id, string(raw))
			}
			if err != nil {
				return err
			}
		}
		for key, state := range edit.Components {
			if state == "inherit" {
				_, err = tx.ExecContext(ctx, "DELETE FROM ufc_event_component_overrides WHERE event_id=$1 AND component_key=$2", id, key)
			} else {
				_, err = tx.ExecContext(ctx, "INSERT INTO ufc_event_component_overrides(event_id,component_key,enabled) VALUES($1,$2,$3) ON CONFLICT(event_id,component_key) DO UPDATE SET enabled=excluded.enabled", id, key, state == "on")
			}
			if err != nil {
				return err
			}
		}
		event.Revision++
		return loadComponentPreferences(ctx, tx, &event)
	})
	if err != nil && (strings.Contains(err.Error(), "database is locked") || strings.Contains(err.Error(), "SQLITE_BUSY")) {
		return catalogue.Event{}, catalogue.ErrRevisionConflict
	}
	return event, aliasWriteError(err)
}

// KnownCatalogueSources supplies persisted revision-cache facts after restart.
func (r *Repository) KnownCatalogueSources(ctx context.Context) ([]catalogue.Facts, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT source_snapshot_json FROM ufc_events WHERE source_page_id IS NOT NULL ORDER BY id LIMIT 2501")
	if err != nil {
		return nil, errors.Wrap(err, "read known catalogue sources")
	}
	defer rows.Close()
	facts := []catalogue.Facts{}
	for rows.Next() {
		var raw string
		var f catalogue.Facts
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(raw), &f); err != nil {
			return nil, errors.Wrap(err, "decode known source")
		}
		facts = append(facts, f)
	}
	if len(facts) > 2500 {
		return nil, errors.New("local catalogue limit exceeded")
	}
	return facts, rows.Err()
}

// CatalogueStatus returns persistent refresh health and bounded unresolved rows.
func (r *Repository) CatalogueStatus(ctx context.Context) (*catalogue.Status, error) {
	if err := r.checkHistory(ctx, r.migrator(), true); err != nil {
		return nil, err
	}
	s := &catalogue.Status{Attribution: "English Wikipedia contributors; source revisions retained with each event", License: "https://creativecommons.org/licenses/by-sa/4.0/"}
	err := r.readTransaction(ctx, func(tx *sql.Tx) error {
		var templates, report string
		var interval int
		err := tx.QueryRowContext(ctx, "SELECT source, periodic_sync_enabled, sync_interval_seconds, component_templates_json, last_attempt, last_success, last_error, last_report_json FROM ufc_catalogue_settings WHERE id=1").Scan(&s.Source, &s.PeriodicSyncEnabled, &interval, &templates, &s.LastAttempt, &s.LastSuccess, &s.LastError, &report)
		if err != nil {
			return errors.Wrap(err, "read catalogue settings")
		}
		if s.Source != "wikipedia.en" || interval < 3600 || interval > 604800 {
			return errors.New("unsupported catalogue settings")
		}
		var componentTemplates catalogue.Templates
		if err := json.Unmarshal([]byte(templates), &componentTemplates); err != nil {
			return errors.Wrap(err, "read local component templates")
		}
		if err := catalogue.ValidateTemplates(componentTemplates); err != nil {
			return err
		}
		if err := json.Unmarshal([]byte(report), &s.Report); err != nil {
			return errors.Wrap(err, "read catalogue report")
		}
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM(CASE WHEN review_json <> '[]' AND review_json <> 'null' THEN 1 ELSE 0 END),0) FROM ufc_events WHERE manual_corrections_json NOT LIKE '%\"deleted\":true%'").Scan(&s.Events, &s.ReviewEvents); err != nil {
			return errors.Wrap(err, "count catalogue events")
		}
		return nil
	})
	return s, err
}

// RecordCatalogueAttempt persists intent without changing the last successful snapshot.
func (r *Repository) RecordCatalogueAttempt(ctx context.Context, at time.Time) error {
	_, err := r.db.ExecContext(ctx, "UPDATE ufc_catalogue_settings SET last_attempt=$1, revision=revision+1 WHERE id=1", at.UTC().Format(time.RFC3339Nano))
	return errors.Wrap(err, "record catalogue attempt")
}

// RecordCatalogueFailure preserves last-good facts and reports a bounded failure summary.
func (r *Repository) RecordCatalogueFailure(ctx context.Context, message string) error {
	if len(message) > 256 {
		return errors.New("catalogue failure summary exceeds limit")
	}
	_, err := r.db.ExecContext(ctx, "UPDATE ufc_catalogue_settings SET last_error=$1, revision=revision+1 WHERE id=1", message)
	return errors.Wrap(err, "record catalogue failure")
}

func (r *Repository) transaction(ctx context.Context, fn func(*sql.Tx) error) error {
	return r.transactionOptions(ctx, nil, fn)
}

func (r *Repository) readTransaction(ctx context.Context, fn func(*sql.Tx) error) error {
	var options *sql.TxOptions
	if r.driver == "postgres" {
		options = &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}
	}
	return r.transactionOptions(ctx, options, fn)
}

func (r *Repository) transactionOptions(ctx context.Context, options *sql.TxOptions, fn func(*sql.Tx) error) error {
	tx, err := r.db.BeginTx(ctx, options)
	if err != nil {
		return errors.Wrap(err, "begin catalogue transaction")
	}
	if err = fn(tx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			r.log.Error().Err(rollbackErr).Msg("could not roll back catalogue transaction")
		}
		return err
	}
	return errors.Wrap(tx.Commit(), "commit catalogue transaction")
}

// ApplyCatalogue commits accepted facts, protected values and health in one transaction.
func (r *Repository) ApplyCatalogue(ctx context.Context, snapshot *catalogue.Snapshot) (catalogue.Report, error) {
	report := catalogue.Report{}
	if snapshot == nil || snapshot.ListRevision <= 0 || snapshot.Discovered < 1 || len(snapshot.Facts) > 2500 || len(snapshot.Diagnostics) > 100 {
		return report, errors.New("invalid or incomplete catalogue snapshot")
	}
	seen := map[int64]bool{}
	for _, f := range snapshot.Facts {
		if err := catalogue.ValidateFacts(f); err != nil {
			return report, err
		}
		if seen[f.PageID] {
			return report, errors.New("duplicate source identity in catalogue snapshot")
		}
		seen[f.PageID] = true
	}
	report = catalogue.Report{AliasTotal: snapshot.AliasTotal, AliasProcessed: snapshot.AliasProcessed, ListRevision: snapshot.ListRevision, Discovered: snapshot.Discovered, Diagnostics: append([]catalogue.Diagnostic{}, snapshot.Diagnostics...), DiagnosticsOmitted: snapshot.DiagnosticsOmitted}
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		for _, source := range snapshot.Facts {
			old, err := decodeEvent(tx.QueryRowContext(ctx, "SELECT "+eventColumns+" FROM ufc_events WHERE source_page_id=$1", source.PageID))
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return errors.Wrap(err, "read event for refresh")
			}
			if old.Corrections.Deleted {
				continue
			}
			if old.ID != "" {
				if err := loadAliases(ctx, tx, &old); err != nil {
					return err
				}
			} else {
				old.ID, err = catalogueIdentity(ctx, tx, source.PageID)
				if err != nil {
					return err
				}
			}
			event, err := catalogue.Merge(old, source)
			if err != nil {
				return err
			}
			for i, a := range event.Aliases {
				if !a.Current || !a.Verified || a.Namespace == "title" {
					continue
				}
				conflict, err := reviewIdentifierOwners(ctx, tx, event.ID, a)
				if err != nil {
					return err
				}
				if conflict {
					event.Aliases[i].Verified = false
					event.Review = appendReview(event.Review, "identifier_owned_by_existing_event")
				}
			}
			if err := saveEvent(ctx, tx, event, old.Revision); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO ufc_event_workspace(event_id) VALUES($1) ON CONFLICT(event_id) DO NOTHING", event.ID); err != nil {
				return err
			}
			report.Accepted++
			if len(event.Review) > 0 {
				report.NeedsReview++
			}
		}
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ufc_events WHERE review_json <> '[]' AND review_json <> 'null' AND manual_corrections_json NOT LIKE '%\"deleted\":true%'").Scan(&report.NeedsReview); err != nil {
			return err
		}
		if snapshot.ListRows != nil {
			if len(snapshot.ListRows) > 2500 || len(snapshot.ListRows) != snapshot.Discovered {
				return errors.New("incomplete full source list")
			}
			raw, err := json.Marshal(snapshot.ListRows)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, "INSERT INTO ufc_catalogue_source_list(id,list_revision,retrieved_at,rows_json) VALUES(1,$1,$2,$3) ON CONFLICT(id) DO UPDATE SET list_revision=excluded.list_revision,retrieved_at=excluded.retrieved_at,rows_json=excluded.rows_json", snapshot.ListRevision, time.Now().UTC().Format(time.RFC3339Nano), string(raw)); err != nil {
				return err
			}
		}
		encoded, err := json.Marshal(report)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE ufc_catalogue_settings SET last_success=$1, last_error='', last_report_json=$2, revision=revision+1 WHERE id=1", time.Now().UTC().Format(time.RFC3339Nano), string(encoded))
		return err
	})
	if err != nil {
		if strings.Contains(err.Error(), "database is locked") || strings.Contains(err.Error(), "SQLITE_BUSY") {
			return catalogue.Report{}, errors.Wrap(catalogue.ErrRevisionConflict, "concurrent catalogue write")
		}
		return catalogue.Report{}, errors.Wrap(err, "apply catalogue snapshot")
	}
	return report, nil
}

func appendReview(reasons []string, reason string) []string {
	for _, v := range reasons {
		if v == reason {
			return reasons
		}
	}
	if len(reasons) < 16 {
		return append(reasons, reason)
	}
	return reasons
}

func reviewIdentifierOwners(ctx context.Context, tx *sql.Tx, id string, alias catalogue.Alias) (bool, error) {
	rows, err := tx.QueryContext(ctx, "SELECT event_id FROM ufc_event_aliases WHERE namespace=$1 AND normalized_value=$2 AND is_current=TRUE AND event_id<>$3", alias.Namespace, catalogue.Normalize(alias.Value), id)
	if err != nil {
		return false, err
	}
	owners := []string{}
	for rows.Next() {
		var owner string
		if err := rows.Scan(&owner); err != nil {
			rows.Close()
			return false, err
		}
		owners = append(owners, owner)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return false, err
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	for _, owner := range owners {
		event, err := decodeEvent(tx.QueryRowContext(ctx, "SELECT "+eventColumns+" FROM ufc_events WHERE id=$1", owner))
		if err != nil {
			return false, err
		}
		if err := loadAliases(ctx, tx, &event); err != nil {
			return false, err
		}
		event.Review = appendReview(event.Review, "identifier_owned_by_existing_event")
		for i, a := range event.Aliases {
			if !a.Manual && a.Namespace == alias.Namespace && catalogue.Normalize(a.Value) == catalogue.Normalize(alias.Value) {
				event.Aliases[i].Verified = false
			}
		}
		if err := saveEvent(ctx, tx, event, event.Revision); err != nil {
			return false, err
		}
	}
	return len(owners) > 0, nil
}

func saveEvent(ctx context.Context, tx *sql.Tx, event catalogue.Event, expected int64) error {
	values := []string{}
	for _, v := range []any{event.Values.Headliners, event.Source, event.Corrections, event.Review} {
		data, err := json.Marshal(v)
		if err != nil {
			return err
		}
		values = append(values, string(data))
	}
	if expected == 0 {
		pageID := sql.NullInt64{Int64: event.Source.PageID, Valid: event.Source.PageID > 0}
		revision := int64(1)
		if pageID.Valid {
			var prior int64
			err := tx.QueryRowContext(ctx, "SELECT last_revision FROM ufc_event_identities WHERE source_page_id=$1", pageID.Int64).Scan(&prior)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if err == nil {
				revision = prior + 1
			}
		}
		_, err := tx.ExecContext(ctx, "INSERT INTO ufc_events (id, revision, event_type, name, event_date, headliners_json, status, source_page_id, source_snapshot_json, manual_corrections_json, review_json) VALUES ($1,$11,$2,$3,$4,$5,$6,$7,$8,$9,$10)", event.ID, event.Values.Type, event.Values.Name, event.Values.Date, values[0], event.Values.Status, pageID, values[1], values[2], values[3], revision)
		if err != nil {
			return errors.Wrap(err, "insert catalogue event")
		}
	} else {
		result, err := tx.ExecContext(ctx, "UPDATE ufc_events SET revision=revision+1, event_type=$1, name=$2, event_date=$3, headliners_json=$4, status=$5, source_snapshot_json=$6, manual_corrections_json=$7, review_json=$8 WHERE id=$9 AND revision=$10", event.Values.Type, event.Values.Name, event.Values.Date, values[0], event.Values.Status, values[1], values[2], values[3], event.ID, expected)
		if err != nil {
			return errors.Wrap(err, "update catalogue event")
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return catalogue.ErrRevisionConflict
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM ufc_event_aliases WHERE event_id=$1", event.ID); err != nil {
		return err
	}
	for _, a := range event.Aliases {
		_, err := tx.ExecContext(ctx, "INSERT INTO ufc_event_aliases(event_id, namespace, value, normalized_value, is_current, verified, manual) VALUES($1,$2,$3,$4,$5,$6,$7)", event.ID, a.Namespace, a.Value, catalogue.Normalize(a.Value), a.Current, a.Verified, a.Manual)
		if err != nil {
			return errors.Wrap(err, "save catalogue alias")
		}
	}
	return nil
}

// CorrectCatalogueEvent saves complete internal correction snapshots and local preferences.
func (r *Repository) CorrectCatalogueEvent(ctx context.Context, id string, expected int64, corrections catalogue.Corrections, aliases []catalogue.Alias, overrides map[string]bool) error {
	for _, a := range aliases {
		if !a.Manual {
			return errors.New("correction aliases must have manual ownership")
		}
		if err := catalogue.ValidateAlias(a); err != nil {
			return err
		}
	}
	if err := catalogue.ValidateOverrides(overrides); err != nil {
		return err
	}
	return r.transaction(ctx, func(tx *sql.Tx) error {
		old, err := decodeEvent(tx.QueryRowContext(ctx, "SELECT "+eventColumns+" FROM ufc_events WHERE id=$1", id))
		if err != nil {
			return err
		}
		if old.Revision != expected {
			return catalogue.ErrRevisionConflict
		}
		if err := loadAliases(ctx, tx, &old); err != nil {
			return err
		}
		old.Corrections = corrections
		for _, a := range aliases {
			old.Aliases = append(old.Aliases, a)
		}
		event, err := catalogue.Merge(old, old.Source)
		if err != nil {
			return err
		}
		if err := saveEvent(ctx, tx, event, expected); err != nil {
			return err
		}
		for key, on := range overrides {
			_, err := tx.ExecContext(ctx, "INSERT INTO ufc_event_component_overrides(event_id,component_key,enabled) VALUES($1,$2,$3) ON CONFLICT(event_id,component_key) DO UPDATE SET enabled=excluded.enabled", id, key, on)
			if err != nil {
				return err
			}
		}
		return nil
	})
}
