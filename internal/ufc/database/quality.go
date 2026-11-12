// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/pkg/errors"
)

func loadQualityPreferences(ctx context.Context, q queryer, e *catalogue.Event) error {
	p := &catalogue.QualityPreferences{}
	var raw string
	if err := q.QueryRowContext(ctx, "SELECT resolutions_json,revision FROM ufc_catalogue_settings WHERE id=1").Scan(&raw, &p.SettingsRevision); err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(raw), &p.Defaults); err != nil {
		return err
	}
	if err := catalogue.ValidateResolutions(p.Defaults); err != nil {
		return err
	}
	err := q.QueryRowContext(ctx, "SELECT resolutions_json FROM ufc_event_quality_overrides WHERE event_id=$1", e.ID).Scan(&raw)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	p.Effective = append([]string{}, p.Defaults...)
	if err == nil {
		if err := json.Unmarshal([]byte(raw), &p.Override); err != nil {
			return err
		}
		if err := catalogue.ValidateResolutions(p.Override); err != nil {
			return err
		}
		p.Effective = append([]string{}, p.Override...)
	}
	e.Quality = p
	return nil
}
