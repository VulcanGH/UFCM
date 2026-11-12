// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package database

import (
	"context"
	"database/sql"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/pkg/errors"
)

// CatalogueMatchingEvents reads active local facts and aliases from one snapshot.
func (r *Repository) CatalogueMatchingEvents(ctx context.Context) ([]catalogue.Event, error) {
	result := []catalogue.Event{}
	err := r.readTransaction(ctx, func(tx *sql.Tx) error {
		rows, err := tx.QueryContext(ctx, "SELECT "+eventColumns+" FROM ufc_events ORDER BY id LIMIT 2501")
		if err != nil {
			return err
		}
		defer rows.Close()
		count := 0
		for rows.Next() {
			count++
			if count > 2500 {
				return errors.New("catalogue exceeds matching preview limit")
			}
			event, err := decodeEvent(rows)
			if err != nil {
				return err
			}
			if !event.Corrections.Deleted {
				result = append(result, event)
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if err := rows.Close(); err != nil {
			return err
		}
		for i := range result {
			if err := loadAliases(ctx, tx, &result[i]); err != nil {
				return err
			}
			if err := loadWorkspaceState(ctx, tx, &result[i]); err != nil {
				return err
			}
		}
		return nil
	})
	return result, errors.Wrap(err, "read matching catalogue")
}
