package database

import (
	"context"
	"database/sql"
	"slices"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/pkg/errors"
)

const identifierConflict = "identifier_owned_by_existing_event"

// ReconcileCatalogueReviews updates the existing database after upgrading,
// including historical records outside the normal refresh window.
func (r *Repository) ReconcileCatalogueReviews(ctx context.Context) error {
	return r.transaction(ctx, func(tx *sql.Tx) error { return reconcileCatalogueReviews(ctx, tx) })
}

func reconcileCatalogueReviews(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, `SELECT `+eventColumns+` FROM ufc_events
        WHERE review_json LIKE '%identifier_owned_by_existing_event%'
           OR review_json LIKE '%competing_event_identifier%'
           OR id IN (SELECT a.event_id FROM ufc_event_aliases a
               JOIN ufc_event_aliases b ON a.namespace=b.namespace AND a.normalized_value=b.normalized_value AND a.event_id<>b.event_id
               WHERE a.is_current=TRUE AND b.is_current=TRUE AND a.namespace<>'title')
        ORDER BY id`)
	if err != nil {
		return err
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
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}

	owners := map[catalogue.AliasKey]int{}
	for i := range events {
		event := &events[i]
		if err := loadAliases(ctx, tx, event); err != nil {
			return err
		}
		for _, a := range event.Aliases {
			if a.Current && a.Namespace != "title" {
				owners[catalogue.KeyForAlias(a)]++
			}
		}
	}
	for _, event := range events {
		review := make([]string, 0, len(event.Review))
		for _, reason := range event.Review {
			if reason != identifierConflict && reason != "competing_event_identifier" {
				review = append(review, reason)
			}
		}
		aliasesChanged := false
		for i, a := range event.Aliases {
			if !a.Current || a.Namespace == "title" {
				continue
			}
			if owners[catalogue.KeyForAlias(a)] > 1 {
				review = appendReview(review, identifierConflict)
				if !a.Manual && a.Verified {
					event.Aliases[i].Verified = false
					aliasesChanged = true
				}
			} else if !a.Manual && !a.Verified && len(review) == 0 &&
				(slices.Contains(event.Review, identifierConflict) || slices.Contains(event.Review, "competing_event_identifier")) {
				for _, source := range event.Source.Aliases {
					if source.Current && source.Verified && catalogue.KeyForAlias(a) == catalogue.KeyForAlias(source) {
						event.Aliases[i].Verified = true
						aliasesChanged = true
						break
					}
				}
			}
		}
		if aliasesChanged || !slices.Equal(review, event.Review) {
			event.Review = review
			if err := saveEvent(ctx, tx, event, event.Revision); err != nil {
				return err
			}
		}
	}
	return nil
}

func conflictRecords(ctx context.Context, tx *sql.Tx, key catalogue.AliasKey) ([]catalogue.Event, error) {
	rows, err := tx.QueryContext(ctx, `SELECT `+eventColumns+` FROM ufc_events
        WHERE id IN (SELECT event_id FROM ufc_event_aliases WHERE namespace=$1 AND normalized_value=$2 AND is_current=TRUE)
        ORDER BY id`, key.Namespace, key.Value)
	if err != nil {
		return nil, err
	}
	var events []catalogue.Event
	for rows.Next() {
		e, err := decodeEvent(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if !e.Corrections.Deleted {
			events = append(events, e)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range events {
		if err := loadAliases(ctx, tx, &events[i]); err != nil {
			return nil, err
		}
	}
	return events, nil
}

func (r *Repository) ResolveAliasConflict(ctx context.Context, id string, choice catalogue.ReviewChoice, actor string) error {
	return r.transaction(ctx, func(tx *sql.Tx) error {
		if choice.Namespace == "title" || choice.Keep == "" || len(choice.Records) < 2 || len(choice.Records) > 32 {
			return catalogue.ErrInvalidEdit
		}
		records, err := conflictRecords(ctx, tx, choice.AliasKey)
		if err != nil {
			return err
		}
		if len(records) != len(choice.Records) {
			return catalogue.ErrRevisionConflict
		}
		revisions := map[string]int64{}
		for _, record := range choice.Records {
			revisions[record.ID] = record.Revision
		}
		var winner int = -1
		for i, event := range records {
			if revisions[event.ID] != event.Revision {
				return catalogue.ErrRevisionConflict
			}
			if choice.FromSource && slices.ContainsFunc(event.Aliases, func(a catalogue.Alias) bool {
				return a.Current && a.Manual && catalogue.KeyForAlias(a) == choice.AliasKey
			}) {
				return errors.Wrap(catalogue.ErrInvalidEdit, "a protected alias is involved; choose which record keeps it")
			}
			if event.ID == choice.Keep {
				winner = i
			}
		}
		if winner < 0 || revisions[id] < 1 {
			return catalogue.ErrInvalidEdit
		}
		// Release the claim everywhere else before verifying the selected owner.
		now := time.Now()
		for i, event := range records {
			if i == winner {
				continue
			}
			event.DecideAlias(choice.AliasKey, false, actor, now)
			if err := saveEvent(ctx, tx, event, event.Revision); err != nil {
				return err
			}
		}
		event := records[winner]
		event.DecideAlias(choice.AliasKey, true, actor, now)
		if err := saveEvent(ctx, tx, event, event.Revision); err != nil {
			return err
		}
		return reconcileCatalogueReviews(ctx, tx)
	})
}
