// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

//go:build integration

package database

import (
	"database/sql"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestCatalogueEditsBothEngines(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := uint32(listener.Addr().(*net.TCPAddr).Port)
	require.NoError(t, listener.Close())
	cache, err := os.UserCacheDir()
	require.NoError(t, err)
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().Port(port).Version(embeddedpostgres.V17).Locale("C").Encoding("UTF8").Username("postgres").Password("edits-test-only").Database("edits").CachePath(filepath.Join(cache, "ufc-foundation", "archive")).BinariesPath(filepath.Join(cache, "ufc-foundation", "binaries")).RuntimePath(filepath.Join(t.TempDir(), "runtime")))
	require.NoError(t, pg.Start())
	t.Cleanup(func() { require.NoError(t, pg.Stop()) })
	for _, engine := range []string{"sqlite", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			dsn := filepath.Join(t.TempDir(), "edits.db")
			if engine == "postgres" {
				dsn = fmt.Sprintf("postgres://postgres:edits-test-only@127.0.0.1:%d/edits?sslmode=disable", port)
			}
			db, err := sql.Open(engine, dsn)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			repo := New(db, engine, zerolog.Nop())
			require.NoError(t, repo.Migrate(t.Context()))
			date := "2027-01-30"
			fact := catalogue.Facts{PageID: 336, ListRevision: 1, ArticleRevision: 1, RetrievedAt: time.Now(), Title: "UFC 336", Name: "Source event", Date: &date, Headliners: []string{"Source One", "Source Two"}, Type: "numbered", Status: "scheduled", Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "336", Current: true, Verified: true}}}
			snapshot := &catalogue.Snapshot{ListRevision: 1, Discovered: 1, Facts: []catalogue.Facts{fact}}
			_, err = repo.ApplyCatalogue(t.Context(), snapshot)
			require.NoError(t, err)
			list, err := repo.CatalogueEvents(t.Context(), "2026-11-10")
			require.NoError(t, err)
			id := list.Upcoming[0].ID
			_, err = db.Exec("INSERT INTO ufc_event_component_overrides(event_id,component_key,enabled) VALUES($1,'embedded',TRUE)", id)
			require.NoError(t, err)
			name, status := "Manual event", "postponed"
			edit := catalogue.EventEdit{Revision: 1, Name: &catalogue.FieldEdit{Operation: "set", Value: &name}, Date: &catalogue.FieldEdit{Operation: "set"}, Headliners: &catalogue.FieldEdit{Operation: "set"}, Status: &catalogue.FieldEdit{Operation: "set", Value: &status}}
			saved, err := repo.EditCatalogueEvent(t.Context(), id, edit, "user:test")
			require.NoError(t, err)
			require.EqualValues(t, 2, saved.Revision)
			require.True(t, saved.Corrections.Date.Protected)
			_, err = repo.EditCatalogueEvent(t.Context(), id, edit, "stale")
			require.ErrorIs(t, err, catalogue.ErrRevisionConflict)
			unchanged, err := repo.LoadCatalogueEvent(t.Context(), id)
			require.NoError(t, err)
			require.Equal(t, saved, unchanged)
			fact.Name = "Latest Wikipedia name"
			fact.Status = "completed"
			fact.ArticleRevision++
			snapshot.Facts[0] = fact
			_, err = repo.ApplyCatalogue(t.Context(), snapshot)
			require.NoError(t, err)
			after, err := New(db, engine, zerolog.Nop()).LoadCatalogueEvent(t.Context(), id)
			require.NoError(t, err)
			require.Equal(t, saved.Values, after.Values)
			require.Equal(t, saved.Corrections, after.Corrections)
			require.Equal(t, fact.Name, after.Source.Name)
			reset := catalogue.EventEdit{Revision: after.Revision, Name: &catalogue.FieldEdit{Operation: "reset"}, Date: &catalogue.FieldEdit{Operation: "reset"}, Headliners: &catalogue.FieldEdit{Operation: "reset"}, Status: &catalogue.FieldEdit{Operation: "reset"}}
			restored, err := repo.EditCatalogueEvent(t.Context(), id, reset, "user:test")
			require.NoError(t, err)
			require.Equal(t, id, restored.ID)
			require.Equal(t, fact.Name, restored.Values.Name)
			require.Equal(t, fact.Date, restored.Values.Date)
			require.Equal(t, fact.Headliners, restored.Values.Headliners)
			require.Equal(t, fact.Status, restored.Values.Status)
			require.Equal(t, catalogue.Corrections{}, restored.Corrections)
			require.Equal(t, after.Aliases, restored.Aliases)
			var enabled bool
			require.NoError(t, db.QueryRow("SELECT enabled FROM ufc_event_component_overrides WHERE event_id=$1 AND component_key='embedded'", id).Scan(&enabled))
			require.True(t, enabled)
			bad := ""
			_, err = repo.EditCatalogueEvent(t.Context(), id, catalogue.EventEdit{Revision: restored.Revision, Name: &catalogue.FieldEdit{Operation: "set", Value: &bad}}, "test")
			require.ErrorIs(t, err, catalogue.ErrInvalidEdit)
			_, err = repo.LoadCatalogueEvent(t.Context(), "missing")
			require.ErrorIs(t, err, catalogue.ErrEventNotFound)
			testComponentPreferences(t, repo, id, snapshot)
		})
	}
}

func testComponentPreferences(t *testing.T, repo *Repository, id string, snapshot *catalogue.Snapshot) {
	ctx := t.Context()
	settings, err := repo.CataloguePreferences(ctx)
	require.NoError(t, err)
	require.Equal(t, catalogue.DefaultTemplates(), settings.Templates)
	require.Equal(t, 1, settings.EventCounts["numbered"])
	event, err := repo.LoadCatalogueEvent(ctx, id)
	require.NoError(t, err)
	require.Equal(t, []string{"main_card", "prelims", "early_prelims"}, event.Components.Defaults)
	saved, err := repo.EditCatalogueEvent(ctx, id, catalogue.EventEdit{Revision: event.Revision, Components: map[string]string{"main_card": "off", "prelims": "off", "embedded": "on"}}, "user:test")
	require.NoError(t, err)
	require.Equal(t, event.Values, saved.Values)
	require.Equal(t, event.Corrections, saved.Corrections)
	_, err = repo.EditCatalogueEvent(ctx, id, catalogue.EventEdit{Revision: event.Revision, Components: map[string]string{"embedded": "off"}}, "stale")
	require.ErrorIs(t, err, catalogue.ErrRevisionConflict)
	templates := catalogue.Templates{"numbered": {"main_card", "prelims", "countdown"}, "fight_night": {"main_card", "prelims", "weigh_ins"}}
	updated, err := repo.SaveCataloguePreferences(ctx, catalogue.SettingsEdit{Revision: settings.Revision, Templates: templates})
	require.NoError(t, err)
	require.Equal(t, settings.Revision+1, updated.Revision)
	_, err = repo.SaveCataloguePreferences(ctx, catalogue.SettingsEdit{Revision: settings.Revision, Templates: catalogue.DefaultTemplates()})
	require.ErrorIs(t, err, catalogue.ErrRevisionConflict)
	after, err := repo.LoadCatalogueEvent(ctx, id)
	require.NoError(t, err)
	require.Equal(t, saved.Revision, after.Revision, "template changes do not rewrite event revisions")
	require.Equal(t, saved.Components.Overrides, after.Components.Overrides)
	require.Equal(t, []string{"countdown", "embedded"}, after.Components.Wanted)
	inherited, err := repo.EditCatalogueEvent(ctx, id, catalogue.EventEdit{Revision: after.Revision, Components: map[string]string{"prelims": "inherit"}}, "user:test")
	require.NoError(t, err)
	require.Equal(t, []string{"prelims", "countdown", "embedded"}, inherited.Components.Wanted)
	require.NotContains(t, inherited.Components.Overrides, "prelims")
	_, err = repo.ApplyCatalogue(ctx, snapshot)
	require.NoError(t, err)
	restarted := New(repo.db, repo.driver, zerolog.Nop())
	after, err = restarted.LoadCatalogueEvent(ctx, id)
	require.NoError(t, err)
	require.Equal(t, inherited.Components, after.Components)
	require.Equal(t, inherited.Corrections, after.Corrections)
	before := snapshotEvents(t, repo)
	for _, choices := range []map[string]string{{"unknown": "on"}, {"prelims": "invalid"}} {
		_, err := repo.EditCatalogueEvent(ctx, id, catalogue.EventEdit{Revision: after.Revision, Components: choices}, "invalid")
		require.ErrorIs(t, err, catalogue.ErrInvalidEdit)
	}
	require.Equal(t, before, snapshotEvents(t, repo))
	failureTrigger := "CREATE TRIGGER ufc_test_override_failure BEFORE INSERT ON ufc_event_component_overrides WHEN NEW.component_key='weigh_ins' BEGIN SELECT RAISE(ABORT,'injected override failure'); END"
	cleanupTrigger := "DROP TRIGGER ufc_test_override_failure"
	if repo.driver == "postgres" {
		failureTrigger = "CREATE FUNCTION ufc_test_fail_override() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.component_key='weigh_ins' THEN RAISE EXCEPTION 'injected override failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER ufc_test_override_failure BEFORE INSERT ON ufc_event_component_overrides FOR EACH ROW EXECUTE FUNCTION ufc_test_fail_override()"
		cleanupTrigger = "DROP TRIGGER ufc_test_override_failure ON ufc_event_component_overrides; DROP FUNCTION ufc_test_fail_override()"
	}
	_, err = repo.db.Exec(failureTrigger)
	require.NoError(t, err)
	failedName := "Must roll back"
	_, err = repo.EditCatalogueEvent(ctx, id, catalogue.EventEdit{Revision: after.Revision, Name: &catalogue.FieldEdit{Operation: "set", Value: &failedName}, Components: map[string]string{"weigh_ins": "on"}}, "test")
	require.Error(t, err)
	_, err = repo.db.Exec(cleanupTrigger)
	require.NoError(t, err)
	require.Equal(t, before, snapshotEvents(t, repo), "component failure rolls back metadata and event revision")
	rolledBack, err := repo.LoadCatalogueEvent(ctx, id)
	require.NoError(t, err)
	require.Equal(t, after.Components, rolledBack.Components)
	latest, err := repo.CataloguePreferences(ctx)
	require.NoError(t, err)
	for _, invalid := range []catalogue.Templates{{"numbered": {"main_card"}}, {"numbered": {"main_card", "main_card"}, "fight_night": {}}, {"numbered": {"unknown"}, "fight_night": {}}} {
		_, err = repo.SaveCataloguePreferences(ctx, catalogue.SettingsEdit{Revision: latest.Revision, Templates: invalid})
		require.ErrorIs(t, err, catalogue.ErrInvalidEdit)
	}
	current, err := repo.CataloguePreferences(ctx)
	require.NoError(t, err)
	require.Equal(t, latest, current)
	var periodic bool
	require.NoError(t, repo.db.QueryRow("SELECT periodic_sync_enabled FROM ufc_catalogue_settings WHERE id=1").Scan(&periodic))
	require.False(t, periodic)
	fact := snapshot.Facts[0]
	fact.PageID = 337
	fact.Title, fact.Name, fact.Type = "UFC Fight Night: Test", "Fight Night test", "fight_night"
	fact.Aliases = nil
	_, err = repo.ApplyCatalogue(ctx, &catalogue.Snapshot{ListRevision: 2, Discovered: 1, Facts: []catalogue.Facts{fact}})
	require.NoError(t, err)
	var fightNightID string
	require.NoError(t, repo.db.QueryRow("SELECT id FROM ufc_events WHERE source_page_id=337").Scan(&fightNightID))
	fightNight, err := repo.LoadCatalogueEvent(ctx, fightNightID)
	require.NoError(t, err)
	require.Equal(t, []string{"main_card", "prelims", "weigh_ins"}, fightNight.Components.Wanted)
	require.Empty(t, fightNight.Components.Overrides)
}
