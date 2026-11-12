// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package database

import (
	"context"
	"crypto/rand"
	"database/sql"

	"github.com/autobrr/autobrr/pkg/errors"
)

func catalogueIdentity(ctx context.Context, q queryer, pageID int64) (string, error) {
	var id string
	err := q.QueryRowContext(ctx, "SELECT event_id FROM ufc_event_identities WHERE source_page_id=$1", pageID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return rand.Text(), nil
	}
	return id, err
}

// ResetCatalogueEvents erases catalogue metadata while retaining external identity links.
func (r *Repository) ResetCatalogueEvents(ctx context.Context) error {
	return r.transaction(ctx, func(tx *sql.Tx) error {
		for _, statement := range []string{
			"INSERT INTO ufc_event_identities(event_id,source_page_id,last_revision) SELECT id,source_page_id,revision FROM ufc_events WHERE source_page_id IS NOT NULL ON CONFLICT(source_page_id) DO UPDATE SET event_id=excluded.event_id,last_revision=excluded.last_revision",
			"DELETE FROM ufc_event_workspace",
			"DELETE FROM ufc_automation_events",
			"DELETE FROM ufc_automation_inbox",
			"UPDATE ufc_application_settings SET automatic_acquisition_enabled=FALSE,revision=revision+1 WHERE id=1",
			"DELETE FROM ufc_event_quality_overrides",
			"DELETE FROM ufc_event_component_overrides",
			"DELETE FROM ufc_event_aliases",
			"DELETE FROM ufc_events",
			"DELETE FROM ufc_catalogue_source_list",
			"UPDATE ufc_catalogue_settings SET periodic_sync_enabled=FALSE,next_due=NULL,last_attempt=NULL,last_success=NULL,last_error='',last_report_json='{}',revision=revision+1 WHERE id=1",
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return errors.Wrap(err, "reset catalogue metadata")
			}
		}
		return nil
	})
}
