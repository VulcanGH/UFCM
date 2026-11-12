// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"strings"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/pkg/errors"
)

// CreateCatalogueEvent creates a protected local event without a source reference.
func (r *Repository) CreateCatalogueEvent(ctx context.Context, input catalogue.EventCreate, actor string) (catalogue.Event, error) {
	event, err := catalogue.ManualEvent(input, actor, time.Now())
	if err != nil {
		return catalogue.Event{}, err
	}
	event.ID = rand.Text()
	err = r.transaction(ctx, func(tx *sql.Tx) error {
		if err := checkAliasOwners(ctx, tx, event); err != nil {
			return err
		}
		if err := saveEvent(ctx, tx, event, 0); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO ufc_event_workspace(event_id) VALUES($1) ON CONFLICT(event_id) DO NOTHING", event.ID); err != nil {
			return err
		}
		return loadComponentPreferences(ctx, tx, &event)
	})
	return event, aliasWriteError(err)
}

func checkAliasOwners(ctx context.Context, q queryer, event catalogue.Event) error {
	for _, a := range event.Aliases {
		if a.Namespace == "title" || !a.Current || !a.Verified {
			continue
		}
		var count int
		if err := q.QueryRowContext(ctx, "SELECT COUNT(*) FROM ufc_event_aliases WHERE namespace=$1 AND normalized_value=$2 AND is_current=TRUE AND verified=TRUE AND event_id<>$3", a.Namespace, catalogue.Normalize(a.Value), event.ID).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			return errors.Wrap(catalogue.ErrInvalidEdit, "verified numbering alias already belongs to another event; review its owner")
		}
	}
	return nil
}

func aliasWriteError(err error) error {
	if err != nil && (strings.Contains(err.Error(), "ufc_current_number_owner") || strings.Contains(err.Error(), "UNIQUE constraint failed: ufc_event_aliases.namespace")) {
		return errors.Wrap(catalogue.ErrInvalidEdit, "verified numbering alias already belongs to another event")
	}
	return err
}

// ClearCatalogueEvents clears the active view without deleting permanent identities.
func (r *Repository) ClearCatalogueEvents(ctx context.Context) error {
	return r.transaction(ctx, func(tx *sql.Tx) error {
		for _, statement := range []string{
			"DELETE FROM ufc_event_workspace",
			"UPDATE ufc_catalogue_settings SET revision=revision+1 WHERE id=1",
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return errors.Wrap(err, "clear event selection")
			}
		}
		return nil
	})
}
