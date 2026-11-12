// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package database

import (
	"context"
	"database/sql"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/pkg/errors"
)

// CatalogueEvents reads both groups and their aliases from one local snapshot.
func (r *Repository) CatalogueEvents(ctx context.Context, today string) (*catalogue.EventList, error) {
	if _, err := time.Parse("2006-01-02", today); err != nil {
		return nil, errors.New("invalid catalogue calendar day")
	}
	result := &catalogue.EventList{Upcoming: []catalogue.EventSummary{}, RecentCompleted: []catalogue.EventSummary{}}
	err := r.readTransaction(ctx, func(tx *sql.Tx) error {
		groups := []struct {
			query string
			args  []any
			dest  *[]catalogue.EventSummary
		}{
			{"SELECT " + eventColumns + " FROM ufc_events WHERE id IN (SELECT event_id FROM ufc_event_workspace) AND ((status IN ('scheduled','postponed') AND (event_date IS NULL OR event_date >= $1)) OR id IN (SELECT event_id FROM ufc_automation_events)) ORDER BY (event_date IS NULL), event_date, id", []any{today}, &result.Upcoming},
			{"SELECT " + eventColumns + " FROM ufc_events WHERE id IN (SELECT event_id FROM ufc_event_workspace) AND (status='completed' OR id IN (SELECT event_id FROM ufc_automation_events)) ORDER BY (event_date IS NULL), event_date DESC, id", nil, &result.RecentCompleted},
		}
		for _, group := range groups {
			rows, err := tx.QueryContext(ctx, group.query, group.args...)
			if err != nil {
				return errors.Wrap(err, "read catalogue events")
			}
			var events []catalogue.Event
			for rows.Next() {
				e, err := decodeEvent(rows)
				if err != nil {
					rows.Close()
					return err
				}
				if !e.Corrections.Deleted {
					events = append(events, e)
				}
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}
			if err := rows.Close(); err != nil {
				return err
			}
			for _, event := range events {
				if group.dest == &result.RecentCompleted && len(*group.dest) >= catalogue.RecentCompletedLimit {
					break
				}
				if err := loadWorkspaceState(ctx, tx, &event); err != nil {
					return err
				}
				if event.WorkspacePast != nil && ((group.dest == &result.Upcoming && *event.WorkspacePast) || (group.dest == &result.RecentCompleted && !*event.WorkspacePast)) {
					continue
				}
				if err := loadAliases(ctx, tx, &event); err != nil {
					return errors.Wrap(err, "read event aliases")
				}
				*group.dest = append(*group.dest, catalogue.Summarize(event))
			}
		}
		return nil
	})
	return result, err
}
