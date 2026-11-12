// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package database

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/pkg/errors"
)

// ClaimCatalogueRefresh advances durable due time before any metadata request.
func (r *Repository) ClaimCatalogueRefresh(ctx context.Context, at time.Time, scheduled bool) (bool, error) {
	claimed := false
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		var revision int64
		var enabled bool
		var interval int
		var next *string
		if err := tx.QueryRowContext(ctx, "SELECT revision, periodic_sync_enabled, sync_interval_seconds, next_due FROM ufc_catalogue_settings WHERE id=1").Scan(&revision, &enabled, &interval, &next); err != nil {
			return err
		}
		if interval < 3600 || interval > 604800 {
			return errors.New("invalid metadata refresh interval")
		}
		if scheduled {
			if !enabled {
				return nil
			}
			if next != nil {
				due, err := time.Parse(time.RFC3339Nano, *next)
				if err != nil {
					return errors.Wrap(err, "invalid metadata refresh due time")
				}
				if at.Before(due) {
					return nil
				}
			}
		}
		var nextDue *string
		if enabled {
			value := at.UTC().Add(time.Duration(interval) * time.Second).Format(time.RFC3339Nano)
			nextDue = &value
		}
		result, err := tx.ExecContext(ctx, "UPDATE ufc_catalogue_settings SET last_attempt=$1, next_due=$2, revision=revision+1 WHERE id=1 AND revision=$3", at.UTC().Format(time.RFC3339Nano), nextDue, revision)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return catalogue.ErrRevisionConflict
		}
		claimed = true
		return nil
	})
	if err != nil && (strings.Contains(err.Error(), "database is locked") || strings.Contains(err.Error(), "SQLITE_BUSY")) {
		return false, catalogue.ErrRevisionConflict
	}
	return claimed, err
}
