// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

//go:build integration

package database

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/pkg/migrator"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestManualSyncUpgradeBothEngines(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := uint32(listener.Addr().(*net.TCPAddr).Port)
	require.NoError(t, listener.Close())
	cache, err := os.UserCacheDir()
	require.NoError(t, err)
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().Port(port).Version(embeddedpostgres.V17).Locale("C").Encoding("UTF8").Username("postgres").Password("completion-test-only").Database("completion").CachePath(filepath.Join(cache, "ufc-foundation", "archive")).BinariesPath(filepath.Join(cache, "ufc-foundation", "binaries")).RuntimePath(filepath.Join(t.TempDir(), "runtime")))
	require.NoError(t, pg.Start())
	t.Cleanup(func() { require.NoError(t, pg.Stop()) })
	for _, engine := range []string{"sqlite", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			ctx := t.Context()
			dsn := filepath.Join(t.TempDir(), "upgrade.db")
			if engine == "postgres" {
				dsn = fmt.Sprintf("postgres://postgres:completion-test-only@127.0.0.1:%d/completion?sslmode=disable", port)
			}
			db, err := sql.Open(engine, dsn)
			require.NoError(t, err)
			db.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			if engine == "sqlite" {
				_, err = db.Exec("PRAGMA foreign_keys=ON")
				require.NoError(t, err)
			}
			old := migrator.NewMigrate(db, migrator.WithEngine(engine), migrator.WithTableName("ufc_schema_migrations"), migrator.WithLegacyConversions(false), migrator.WithEmbedFS(migrations, "migrations/"+engine), migrator.WithLogger(zerolog.Nop()))
			old.AddFileMigration("0_application_settings.sql")
			old.AddFileMigration("2_event_catalogue.sql")
			require.NoError(t, old.InitVersionTable())
			require.NoError(t, old.Migrate())
			repo := New(db, engine, zerolog.Nop())
			f := catalogue.Facts{PageID: 336, ListRevision: 1, RetrievedAt: time.Now().UTC(), Title: "UFC 336", Name: "UFC 336", Type: "numbered", Status: "scheduled", Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "336", Current: true, Verified: true}}}
			e, err := catalogue.Merge(catalogue.Event{ID: "keep-this-id"}, f)
			require.NoError(t, err)
			source, err := json.Marshal(e.Source)
			require.NoError(t, err)
			_, err = db.ExecContext(ctx, "INSERT INTO ufc_events(id,revision,event_type,name,event_date,headliners_json,status,source_page_id,source_snapshot_json,manual_corrections_json,review_json) VALUES($1,1,$2,$3,NULL,'null',$4,$5,$6,'{}','[]')", e.ID, e.Values.Type, e.Values.Name, e.Values.Status, f.PageID, string(source))
			require.NoError(t, err)
			_, err = db.ExecContext(ctx, "INSERT INTO ufc_event_aliases(event_id,namespace,value,normalized_value,is_current,verified,manual) VALUES($1,'ufc.numbered','336','336',TRUE,TRUE,FALSE)", e.ID)
			require.NoError(t, err)
			_, err = db.Exec("INSERT INTO ufc_event_component_overrides(event_id,component_key,enabled) VALUES('keep-this-id','prelims',FALSE)")
			require.NoError(t, err)
			eventsBefore := snapshot(t, db, "ufc_events")
			require.NoError(t, repo.Migrate(ctx))
			require.NoError(t, repo.Migrate(ctx))
			require.Equal(t, eventsBefore, snapshot(t, db, "ufc_events"))
			preserved, err := repo.LoadCatalogueEvent(ctx, e.ID)
			require.NoError(t, err)
			require.Equal(t, []string{"1080p"}, preserved.Quality.Effective)
			custom, err := repo.EditCatalogueEvent(ctx, e.ID, catalogue.EventEdit{Revision: preserved.Revision, Quality: &catalogue.QualityEdit{Mode: "custom", Resolutions: []string{"720p"}}}, "test")
			require.NoError(t, err)
			prefs, err := repo.CataloguePreferences(ctx)
			require.NoError(t, err)
			resolutions := []string{"2160p"}
			updated, err := repo.SaveCataloguePreferences(ctx, catalogue.SettingsEdit{Revision: prefs.Revision, Templates: prefs.Templates, Resolutions: &resolutions})
			require.NoError(t, err)
			require.Equal(t, resolutions, updated.Resolutions)
			_, err = repo.SaveCataloguePreferences(ctx, catalogue.SettingsEdit{Revision: prefs.Revision, Templates: prefs.Templates, Resolutions: &resolutions})
			require.ErrorIs(t, err, catalogue.ErrRevisionConflict)
			custom, err = repo.LoadCatalogueEvent(ctx, e.ID)
			require.NoError(t, err)
			require.Equal(t, []string{"720p"}, custom.Quality.Effective)
			require.Equal(t, f.Aliases, preserved.Aliases)
			require.Equal(t, map[string]bool{"prelims": false}, preserved.Components.Overrides)
			if engine == "sqlite" {
				rows, err := db.Query("PRAGMA foreign_key_check")
				require.NoError(t, err)
				require.False(t, rows.Next())
				require.NoError(t, rows.Close())
			}
			local, err := repo.CreateCatalogueEvent(ctx, catalogue.EventCreate{Values: catalogue.Values{Name: "Local test", Type: "fight_night", Status: "scheduled"}}, "test")
			require.NoError(t, err)
			require.Zero(t, local.Source.PageID)
			require.Nil(t, local.Quality.Override)
			require.Equal(t, resolutions, local.Quality.Effective)
			require.NotEqual(t, e.ID, local.ID)
			_, err = repo.CreateCatalogueEvent(ctx, catalogue.EventCreate{Values: local.Values, Aliases: f.Aliases}, "test")
			require.ErrorIs(t, err, catalogue.ErrInvalidEdit)
			loaded, err := repo.LoadCatalogueEvent(ctx, e.ID)
			require.NoError(t, err)
			typ := "fight_night"
			loaded, err = repo.EditCatalogueEvent(ctx, e.ID, catalogue.EventEdit{Revision: loaded.Revision, Type: &catalogue.FieldEdit{Operation: "set", Value: &typ}, Aliases: &catalogue.AliasEdit{Operation: "set", Values: []catalogue.Alias{}}}, "test")
			require.NoError(t, err)
			_, err = repo.ApplyCatalogue(ctx, &catalogue.Snapshot{ListRevision: 1, Discovered: 1, Facts: []catalogue.Facts{f}})
			require.NoError(t, err)
			loaded, err = repo.LoadCatalogueEvent(ctx, e.ID)
			require.NoError(t, err)
			require.Equal(t, typ, loaded.Values.Type)
			require.Equal(t, []string{"720p"}, loaded.Quality.Effective)
			reset, err := repo.EditCatalogueEvent(ctx, e.ID, catalogue.EventEdit{Revision: loaded.Revision, Quality: &catalogue.QualityEdit{Mode: "inherit"}}, "test")
			require.NoError(t, err)
			require.Nil(t, reset.Quality.Override)
			require.Equal(t, resolutions, reset.Quality.Effective)
			_, err = repo.EditCatalogueEvent(ctx, e.ID, catalogue.EventEdit{Revision: loaded.Revision, Quality: &catalogue.QualityEdit{Mode: "custom", Resolutions: []string{"1080p"}}}, "test")
			require.ErrorIs(t, err, catalogue.ErrRevisionConflict)
			loaded = reset
			require.Empty(t, loaded.Aliases)
			require.Equal(t, false, loaded.Components.Overrides["prelims"])
			_, err = repo.EditCatalogueEvent(ctx, e.ID, catalogue.EventEdit{Revision: loaded.Revision, Aliases: &catalogue.AliasEdit{Operation: "reset"}}, "test")
			require.NoError(t, err)
			s, err := repo.CataloguePreferences(ctx)
			require.NoError(t, err)
			enabled, interval := true, 3600
			s, err = repo.SaveCataloguePreferences(ctx, catalogue.SettingsEdit{Revision: s.Revision, Templates: s.Templates, PeriodicSyncEnabled: &enabled, SyncIntervalSeconds: &interval})
			require.NoError(t, err)
			require.NotNil(t, s.NextDue)
			due, err := time.Parse(time.RFC3339Nano, *s.NextDue)
			require.NoError(t, err)
			claimed, err := repo.ClaimCatalogueRefresh(ctx, due.Add(-time.Second), true)
			require.NoError(t, err)
			require.False(t, claimed)
			claimed, err = repo.ClaimCatalogueRefresh(ctx, due, true)
			require.NoError(t, err)
			require.True(t, claimed)
			require.NoError(t, repo.RecordCatalogueFailure(ctx, "mock failure"))
			claimed, err = repo.ClaimCatalogueRefresh(ctx, due.Add(time.Second), true)
			require.NoError(t, err)
			require.False(t, claimed)
			known, err := repo.KnownCatalogueSources(ctx)
			require.NoError(t, err)
			require.Len(t, known, 1)
		})
	}
}
