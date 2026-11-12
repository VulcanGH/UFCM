// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
)

func libraryWanted(ctx context.Context, q queryer) (map[string][]string, error) {
	var raw string
	if err := q.QueryRowContext(ctx, "SELECT component_templates_json FROM ufc_catalogue_settings WHERE id=1").Scan(&raw); err != nil {
		return nil, err
	}
	var templates catalogue.Templates
	if err := json.Unmarshal([]byte(raw), &templates); err != nil {
		return nil, err
	}
	if err := catalogue.ValidateTemplates(templates); err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT e.id,e.event_type,o.component_key,o.enabled FROM ufc_events e LEFT JOIN ufc_event_component_overrides o ON o.event_id=e.id WHERE e.manual_corrections_json NOT LIKE '%"deleted":true%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	types := map[string]string{}
	overrides := map[string]map[string]bool{}
	for rows.Next() {
		var id, eventType string
		var key sql.NullString
		var enabled sql.NullBool
		if err := rows.Scan(&id, &eventType, &key, &enabled); err != nil {
			return nil, err
		}
		types[id] = eventType
		if key.Valid && enabled.Valid {
			if overrides[id] == nil {
				overrides[id] = map[string]bool{}
			}
			overrides[id][key.String] = enabled.Bool
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	wanted := map[string][]string{}
	for id, eventType := range types {
		wanted[id] = catalogue.Components(eventType, templates, overrides[id])
	}
	return wanted, nil
}
