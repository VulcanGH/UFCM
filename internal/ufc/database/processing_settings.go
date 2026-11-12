// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/processing"
)

// ProcessingSettings reads saved preferences without accessing media paths.
func (r *Repository) ProcessingSettings(ctx context.Context) (*processing.Settings, error) {
	var revision int64
	var raw string
	if err := r.db.QueryRowContext(ctx, "SELECT revision,settings_json FROM ufc_processing_settings WHERE id=1").Scan(&revision, &raw); err != nil {
		return nil, err
	}
	var settings processing.Settings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return nil, err
	}
	settings.Revision = revision
	return &settings, nil
}

// SaveProcessingSettings updates preferences at an expected revision.
func (r *Repository) SaveProcessingSettings(ctx context.Context, settings processing.Settings) (*processing.Settings, error) {
	raw, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	err = r.transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, "UPDATE ufc_processing_settings SET revision=revision+1,settings_json=$1 WHERE id=1 AND revision=$2", string(raw), settings.Revision)
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
	settings.Revision++
	return &settings, nil
}
