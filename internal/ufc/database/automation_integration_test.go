// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

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

	upstream "github.com/autobrr/autobrr/internal/database"
	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/feed"
	"github.com/autobrr/autobrr/internal/ufc/acquisition"
	"github.com/autobrr/autobrr/internal/ufc/automation"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/pkg/migrator"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestAutomationBothEngines(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := uint32(listener.Addr().(*net.TCPAddr).Port)
	require.NoError(t, listener.Close())
	cache, err := os.UserCacheDir()
	require.NoError(t, err)
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().Port(port).Version(embeddedpostgres.V17).Locale("C").Encoding("UTF8").Username("postgres").Password("automation-test-only").Database("automation").CachePath(filepath.Join(cache, "ufc-foundation", "archive")).BinariesPath(filepath.Join(cache, "ufc-foundation", "binaries")).RuntimePath(filepath.Join(t.TempDir(), "runtime")))
	require.NoError(t, pg.Start())
	t.Cleanup(func() { require.NoError(t, pg.Stop()) })
	for _, engine := range []string{"sqlite", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			ctx := t.Context()
			dsn := filepath.Join(t.TempDir(), "automation.db")
			if engine == "postgres" {
				dsn = fmt.Sprintf("postgres://postgres:automation-test-only@127.0.0.1:%d/automation?sslmode=disable", port)
			}
			appDB, err := upstream.NewDB(&domain.Config{DatabaseType: engine, DatabaseDSN: dsn, DatabaseAutoMigrate: true}, zerolog.Nop())
			require.NoError(t, err)
			require.NoError(t, appDB.Open())
			t.Cleanup(func() { require.NoError(t, appDB.Close()) })
			db := appDB.Handler
			repo := New(db, engine, zerolog.Nop())
			old := migrator.NewMigrate(db, migrator.WithEngine(engine), migrator.WithTableName("ufc_schema_migrations"), migrator.WithLegacyConversions(false), migrator.WithEmbedFS(migrations, "migrations/"+engine), migrator.WithLogger(zerolog.Nop()))
			for _, file := range []string{"0_application_settings.sql", "2_event_catalogue.sql", "3_manual_events_sync.sql", "4_quality_preferences.sql", "5_manual_acquisitions.sql", "6_library_inventory.sql", "7_permanent_catalogue.sql", "8_download_visibility.sql", "9_catalogue_identities.sql", "10_manual_workflow.sql", "11_processing_settings.sql", "12_processing_jobs.sql", "13_processing_recovery.sql", "14_processing_file_components.sql", "15_processing_cleanup.sql", "16_processing_handoff.sql"} {
				old.AddFileMigration(file)
			}
			require.NoError(t, old.InitVersionTable())
			require.NoError(t, old.Migrate())
			now := time.Now().UTC()
			date := now.Add(24 * time.Hour).Format("2006-01-02")
			event, err := catalogue.ManualEvent(catalogue.EventCreate{Values: catalogue.Values{Name: "UFC 999", Type: "numbered", Status: "scheduled", Date: &date}}, "test", now)
			require.NoError(t, err)
			event.ID = "upcoming"
			require.NoError(t, repo.transaction(ctx, func(tx *sql.Tx) error { return saveEvent(ctx, tx, event, 0) }))
			_, err = db.Exec("INSERT INTO ufc_event_workspace(event_id) VALUES($1)", event.ID)
			require.NoError(t, err)
			eventsBefore := snapshot(t, db, "ufc_events")
			upstreamBefore := snapshot(t, db, "schema_migrations")
			require.NoError(t, repo.Migrate(ctx))
			require.NoError(t, repo.Migrate(ctx))
			require.Equal(t, eventsBefore, snapshot(t, db, "ufc_events"))
			require.Equal(t, upstreamBefore, snapshot(t, db, "schema_migrations"))
			settings, err := repo.AutomationSettings(ctx)
			require.NoError(t, err)
			require.False(t, settings.Enabled)
			_, err = repo.SaveAutomationSettings(ctx, automation.Settings{Revision: settings.Revision, Enabled: true})
			require.ErrorIs(t, err, catalogue.ErrInvalidEdit)
			_, err = db.Exec(`UPDATE ufc_processing_settings SET settings_json='{"sab_client_id":1,"sab_category":"ufc"}' WHERE id=1`)
			require.NoError(t, err)
			settings, err = repo.SaveAutomationSettings(ctx, automation.Settings{Revision: settings.Revision, Enabled: true})
			require.NoError(t, err)
			_, err = repo.SaveAutomationSettings(ctx, automation.Settings{Revision: settings.Revision - 1, Enabled: false})
			require.ErrorIs(t, err, catalogue.ErrRevisionConflict)
			active, err := repo.AutomationEvents(ctx, now)
			require.NoError(t, err)
			require.Len(t, active, 1)
			item := automation.Item{ID: "inbox", EventID: event.ID, Component: "main_card", FeedID: 1, ConfigKey: "saved", Release: feed.SearchItem{Title: "UFC.999.1080p", URL: "https://fixture.invalid/nzb", Reference: "release"}}
			require.NoError(t, repo.EnqueueAutomationItem(ctx, item))
			require.NoError(t, repo.EnqueueAutomationItem(ctx, item))
			restarted := New(db, engine, zerolog.Nop())
			pending, err := restarted.AutomationItems(ctx, now.Add(time.Minute))
			require.NoError(t, err)
			require.Len(t, pending, 1)
			require.Equal(t, item.Release.URL, pending[0].Release.URL)
			j := acquisition.Job{ID: "automatic", EventID: event.ID, Component: "main_card", EventRevision: active[0].Revision, SettingsRevision: active[0].Quality.SettingsRevision, Title: "UFC.999.1080p", ClientID: 1, Category: "ufc", Key: "one", CreatedAt: now.Format(time.RFC3339Nano), Decision: &acquisition.Decision{Automated: true}}
			reserved, created, err := repo.ReserveAcquisition(ctx, j)
			require.NoError(t, err)
			require.True(t, created)
			_, created, err = repo.ReserveAcquisition(ctx, j)
			require.NoError(t, err)
			require.False(t, created)
			require.NoError(t, repo.FinishAutomationItem(ctx, item.ID, "submitted", "reservation recorded", now))
			pending, err = repo.AutomationItems(ctx, now.Add(time.Minute))
			require.NoError(t, err)
			require.Empty(t, pending)
			settings, err = repo.SaveAutomationSettings(ctx, automation.Settings{Revision: settings.Revision, Enabled: false})
			require.NoError(t, err)
			j.ID = "disabled"
			j.Component = "prelims"
			j.Key = "two"
			_, _, err = repo.ReserveAcquisition(ctx, j)
			require.ErrorIs(t, err, catalogue.ErrInvalidEdit)
			reserved.State = "completed"
			reserved.RemoteID = "mock-sab"
			reserved.Storage = "fixture-storage"
			require.NoError(t, repo.SaveAcquisition(ctx, reserved))
			notices, err := repo.AutomationNotices(ctx, now.Add(time.Minute))
			require.NoError(t, err)
			require.Len(t, notices, 1)
			require.NoError(t, repo.FinishAutomationNotice(ctx, notices[0].ID, false, now.Add(time.Hour)))
			notices, err = repo.AutomationNotices(ctx, now.Add(time.Minute))
			require.NoError(t, err)
			require.Empty(t, notices)
			active, err = repo.AutomationEvents(ctx, now.Add(5*24*time.Hour))
			require.NoError(t, err)
			require.Empty(t, active)
			listing, err := repo.CatalogueEvents(ctx, now.Format("2006-01-02"))
			require.NoError(t, err)
			require.Empty(t, listing.Upcoming)
			require.Len(t, listing.RecentCompleted, 1)
			loaded, err := repo.LoadCatalogueEvent(ctx, event.ID)
			require.NoError(t, err)
			require.Equal(t, "scheduled", loaded.Values.Status)
			_, err = db.Exec("UPDATE ufc_automation_events SET archived_at='' WHERE event_id=$1", event.ID)
			require.NoError(t, err)
			_, err = db.Exec("INSERT INTO ufc_library_roots(id,path,enabled,state) VALUES('fixture-root','fixture-library',TRUE,'available')")
			require.NoError(t, err)
			for i, component := range []string{"main_card", "prelims", "early_prelims"} {
				_, err = db.Exec(`INSERT INTO ufc_library_files(id,root_id,relative_path,size,modified,event_id,component,resolution,state,reason,manual,present,group_key,last_seen,detected_event,detected_component,detected_resolution,detected_state,detected_reason,actor) VALUES($1,'fixture-root',$1,1,'fixture',$2,$3,'1080p','confirmed','fixture',FALSE,TRUE,'event:upcoming','fixture',$2,$3,'1080p','confirmed','fixture','test')`, component, event.ID, component)
				require.NoError(t, err)
				active, err = repo.AutomationEvents(ctx, now)
				require.NoError(t, err)
				if i < 2 {
					require.Len(t, active, 1)
				} else {
					require.Empty(t, active)
				}
			}

			var reason string
			require.NoError(t, db.QueryRow("SELECT reason FROM ufc_automation_events WHERE event_id=$1", event.ID).Scan(&reason))
			require.Equal(t, "wanted_components_complete", reason)
		})
	}
}
