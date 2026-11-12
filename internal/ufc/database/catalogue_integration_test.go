// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

//go:build integration

package database

import (
	"database/sql"
	"testing"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func testCatalogueRepository(t *testing.T, repo *Repository) {
	ctx := t.Context()
	date := "2024-04-13"
	fact := catalogue.Facts{PageID: 300, ArticleRevision: 10, ListRevision: 20, RetrievedAt: time.Now().UTC(), Title: "UFC 300", Name: "UFC 300: Pereira vs. Hill", Type: "numbered", Date: &date, Headliners: []string{"Pereira", "Hill"}, Status: "scheduled", Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "300", Current: true, Verified: true}, {Namespace: "title", Value: "UFC 300: Pereira vs. Hill", Current: true, Verified: true}}, Section: "scheduled events"}
	snapshot := &catalogue.Snapshot{ListRevision: 20, Discovered: 1, Facts: []catalogue.Facts{fact}}
	status, err := repo.CatalogueStatus(ctx)
	require.NoError(t, err)
	require.False(t, status.PeriodicSyncEnabled)
	require.Zero(t, status.Events)
	report, err := repo.ApplyCatalogue(ctx, snapshot)
	require.NoError(t, err)
	require.Equal(t, 1, report.Accepted)
	var id string
	require.NoError(t, repo.db.QueryRow("SELECT id FROM ufc_events WHERE source_page_id=300").Scan(&id))
	require.NotEqual(t, "300", id)
	event, err := repo.LoadCatalogueEvent(ctx, id)
	require.NoError(t, err)
	require.EqualValues(t, 1, event.Revision)
	repo = New(repo.db, repo.driver, zerolog.Nop())
	_, err = repo.ApplyCatalogue(ctx, snapshot)
	require.NoError(t, err)
	require.NoError(t, repo.db.QueryRow("SELECT id FROM ufc_events WHERE source_page_id=300").Scan(&id))
	again, err := repo.LoadCatalogueEvent(ctx, id)
	require.NoError(t, err)
	require.Equal(t, event.ID, again.ID)
	name := "My protected event"
	corrections := catalogue.Corrections{Name: &catalogue.Correction{Protected: true, Value: &name}, Date: &catalogue.Correction{Protected: true, Value: nil}, Headliners: &catalogue.Correction{Protected: true, Names: []string{}}}
	manual := []catalogue.Alias{{Namespace: "title", Value: "My manual title", Manual: true, Current: true, Verified: true}}
	require.NoError(t, repo.CorrectCatalogueEvent(ctx, id, again.Revision, corrections, manual, map[string]bool{"prelims": false, "embedded": true}))
	require.ErrorIs(t, repo.CorrectCatalogueEvent(ctx, id, again.Revision, corrections, nil, nil), catalogue.ErrRevisionConflict)
	newDate := "2024-05-01"
	fact.Name = "UFC 300: New vs. Billing 2"
	fact.Date = &newDate
	fact.Headliners = []string{"New", "Billing"}
	fact.Status = "completed"
	fact.Section = "past events"
	fact.ArticleRevision++
	snapshot.Facts = []catalogue.Facts{fact}
	_, err = repo.ApplyCatalogue(ctx, snapshot)
	require.NoError(t, err)
	event, err = repo.LoadCatalogueEvent(ctx, id)
	require.NoError(t, err)
	require.Equal(t, name, event.Values.Name)
	require.Nil(t, event.Values.Date)
	require.Empty(t, event.Values.Headliners)
	require.Equal(t, "completed", event.Values.Status)
	require.Equal(t, fact.Name, event.Source.Name)
	require.Equal(t, newDate, *event.Source.Date)
	require.Contains(t, event.Aliases, manual[0])
	var enabled bool
	require.NoError(t, repo.db.QueryRow("SELECT enabled FROM ufc_event_component_overrides WHERE event_id=$1 AND component_key='embedded'", id).Scan(&enabled))
	require.True(t, enabled)
	// A delayed refresh uses current corrections, rather than the revision it fetched before the editor.
	known, err := repo.KnownCatalogueSources(ctx)
	require.NoError(t, err)
	require.Len(t, known, 1)
	ready := make(chan struct{})
	applied := make(chan error, 1)
	go func() { <-ready; _, err := repo.ApplyCatalogue(ctx, snapshot); applied <- err }()
	newName := "Concurrent protected value"
	corrections.Name.Value = &newName
	require.NoError(t, repo.CorrectCatalogueEvent(ctx, id, event.Revision, corrections, nil, nil))
	close(ready)
	require.NoError(t, <-applied)
	event, err = repo.LoadCatalogueEvent(ctx, id)
	require.NoError(t, err)
	require.Equal(t, newName, event.Values.Name)
	before := snapshotEvents(t, repo)
	bad := fact
	bad.PageID = 301
	bad.Name = "UFC 301"
	bad.Title = "UFC 301"
	bad.Aliases = []catalogue.Alias{{Namespace: "title", Value: "duplicate", Current: true}, {Namespace: "title", Value: "duplicate", Current: true}}
	failureTrigger := "CREATE TRIGGER ufc_test_insert_failure BEFORE INSERT ON ufc_events WHEN NEW.source_page_id=301 BEGIN SELECT RAISE(ABORT,'injected catalogue write failure'); END"
	cleanupTrigger := "DROP TRIGGER ufc_test_insert_failure"
	if repo.driver == "postgres" {
		failureTrigger = "CREATE FUNCTION ufc_test_fail_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.source_page_id=301 THEN RAISE EXCEPTION 'injected catalogue write failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER ufc_test_insert_failure BEFORE INSERT ON ufc_events FOR EACH ROW EXECUTE FUNCTION ufc_test_fail_insert()"
		cleanupTrigger = "DROP TRIGGER ufc_test_insert_failure ON ufc_events; DROP FUNCTION ufc_test_fail_insert()"
	}
	_, err = repo.db.Exec(failureTrigger)
	require.NoError(t, err)
	_, err = repo.ApplyCatalogue(ctx, &catalogue.Snapshot{ListRevision: 20, Discovered: 2, Facts: []catalogue.Facts{fact, bad}})
	require.Error(t, err)
	require.Equal(t, before, snapshotEvents(t, repo), "mid-transaction failure must roll back prior accepted rows")
	_, err = repo.db.Exec(cleanupTrigger)
	require.NoError(t, err)
	other := fact
	other.PageID = 302
	other.Title = "UFC 302"
	other.Name = "UFC 302"
	other.ArticleRevision = 1
	_, err = repo.ApplyCatalogue(ctx, &catalogue.Snapshot{ListRevision: 20, Discovered: 1, Facts: []catalogue.Facts{other}})
	require.NoError(t, err)
	var otherID string
	require.NoError(t, repo.db.QueryRow("SELECT id FROM ufc_events WHERE source_page_id=302").Scan(&otherID))
	require.NotEqual(t, id, otherID, "same title/date/number cannot merge different page identities")
	otherEvent, err := repo.LoadCatalogueEvent(ctx, otherID)
	require.NoError(t, err)
	require.Contains(t, otherEvent.Review, "identifier_owned_by_existing_event")
	for _, a := range otherEvent.Aliases {
		if a.Namespace == "ufc.numbered" {
			require.False(t, a.Verified)
		}
	}
	event, err = repo.LoadCatalogueEvent(ctx, id)
	require.NoError(t, err)
	require.Contains(t, event.Review, "identifier_owned_by_existing_event")
	for _, a := range event.Aliases {
		if a.Namespace == "ufc.numbered" {
			require.False(t, a.Verified)
		}
	}
	_, err = repo.ApplyCatalogue(ctx, &catalogue.Snapshot{ListRevision: 20, Discovered: 1, Facts: []catalogue.Facts{other}})
	require.NoError(t, err)
	otherEvent, err = repo.LoadCatalogueEvent(ctx, otherID)
	require.NoError(t, err)
	for _, a := range otherEvent.Aliases {
		if a.Namespace == "ufc.numbered" {
			require.False(t, a.Verified, "unresolved ownership cannot become verified on retry")
		}
	}
	status, err = repo.CatalogueStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, status.Events)
	lastSuccess := *status.LastSuccess
	require.NoError(t, repo.RecordCatalogueFailure(ctx, "mock source unavailable"))
	status, err = repo.CatalogueStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, lastSuccess, *status.LastSuccess)
	require.Equal(t, 2, status.Events)
	require.Equal(t, "mock source unavailable", status.LastError)
	_, err = repo.ApplyCatalogue(ctx, &catalogue.Snapshot{ListRevision: 21, Discovered: 1, Diagnostics: []catalogue.Diagnostic{{Name: "Unresolved", Reason: "section_redirect"}}})
	require.NoError(t, err)
	status, err = repo.CatalogueStatus(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, status.Events)
	require.Len(t, status.Report.Diagnostics, 1)
	unicodeEvent := fact
	unicodeEvent.PageID = 999
	unicodeEvent.Title = "UFC Fight Night: Procházka vs. Stirling"
	unicodeEvent.Name = unicodeEvent.Title
	unicodeEvent.Type = "fight_night"
	unicodeEvent.Headliners = []string{"Procházka", "Stirling"}
	unicodeEvent.Aliases = []catalogue.Alias{{Namespace: "title", Value: unicodeEvent.Name, Current: true, Verified: true}, {Namespace: "ufc.fight_night", Value: "294", Current: true, Verified: true}}
	_, err = repo.ApplyCatalogue(ctx, &catalogue.Snapshot{ListRevision: 21, Discovered: 1, Facts: []catalogue.Facts{unicodeEvent}})
	require.NoError(t, err)
	var unicodeID string
	require.NoError(t, repo.db.QueryRow("SELECT id FROM ufc_events WHERE source_page_id=999").Scan(&unicodeID))
	storedUnicode, err := repo.LoadCatalogueEvent(ctx, unicodeID)
	require.NoError(t, err)
	require.Equal(t, unicodeEvent.Name, storedUnicode.Values.Name)
	require.Equal(t, unicodeEvent.Headliners, storedUnicode.Values.Headliners)
	require.Equal(t, unicodeEvent.Name, storedUnicode.Source.Name)
	if repo.driver == "postgres" {
		require.NoError(t, repo.readTransaction(ctx, func(tx *sql.Tx) error {
			var before, after int
			require.NoError(t, tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ufc_events").Scan(&before))
			committed := make(chan error, 1)
			concurrent := fact
			concurrent.PageID = 303
			concurrent.Title = "UFC 303"
			concurrent.Name = "UFC 303"
			concurrent.Aliases = []catalogue.Alias{{Namespace: "ufc.numbered", Value: "303", Current: true, Verified: true}}
			go func() {
				_, err := repo.ApplyCatalogue(ctx, &catalogue.Snapshot{ListRevision: 21, Discovered: 1, Facts: []catalogue.Facts{concurrent}})
				committed <- err
			}()
			require.NoError(t, <-committed)
			require.NoError(t, tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ufc_events").Scan(&after))
			require.Equal(t, before, after, "status must retain one snapshot across a concurrent commit")
			return nil
		}))
	}
}

func snapshotEvents(t *testing.T, repo *Repository) string {
	t.Helper()
	return snapshot(t, repo.db, "ufc_events")
}
