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

	upstream "github.com/autobrr/autobrr/internal/database"
	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/ufc/acquisition"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/processing"
	"github.com/autobrr/autobrr/pkg/migrator"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestManualDownloadsBothEngines(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := uint32(listener.Addr().(*net.TCPAddr).Port)
	require.NoError(t, listener.Close())
	cache, err := os.UserCacheDir()
	require.NoError(t, err)
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().Port(port).Version(embeddedpostgres.V17).Locale("C").Encoding("UTF8").Username("postgres").Password("downloads-test-only").Database("downloads").CachePath(filepath.Join(cache, "ufc-foundation", "archive")).BinariesPath(filepath.Join(cache, "ufc-foundation", "binaries")).RuntimePath(filepath.Join(t.TempDir(), "runtime")))
	require.NoError(t, pg.Start())
	t.Cleanup(func() { require.NoError(t, pg.Stop()) })
	for _, engine := range []string{"sqlite", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			ctx := t.Context()
			dsn := filepath.Join(t.TempDir(), "downloads.db")
			if engine == "postgres" {
				dsn = fmt.Sprintf("postgres://postgres:downloads-test-only@127.0.0.1:%d/downloads?sslmode=disable", port)
			}
			appDB, err := upstream.NewDB(&domain.Config{DatabaseType: engine, DatabaseDSN: dsn, DatabaseAutoMigrate: true}, zerolog.Nop())
			require.NoError(t, err)
			require.NoError(t, appDB.Open())
			t.Cleanup(func() { require.NoError(t, appDB.Close()) })
			db := appDB.Handler
			_, err = db.Exec("INSERT INTO indexer(id,name,identifier) VALUES(1001,'Saved source','saved-source')")
			require.NoError(t, err)
			history := snapshot(t, db, "schema_migrations")
			indexers := snapshot(t, db, "indexer")
			old := migrator.NewMigrate(db, migrator.WithEngine(engine), migrator.WithTableName("ufc_schema_migrations"), migrator.WithLegacyConversions(false), migrator.WithEmbedFS(migrations, "migrations/"+engine), migrator.WithLogger(zerolog.Nop()))
			for _, file := range []string{"0_application_settings.sql", "2_event_catalogue.sql", "3_manual_events_sync.sql", "4_quality_preferences.sql"} {
				old.AddFileMigration(file)
			}
			require.NoError(t, old.InitVersionTable())
			require.NoError(t, old.Migrate())
			repo := New(db, engine, zerolog.Nop())
			event, err := catalogue.ManualEvent(catalogue.EventCreate{Values: catalogue.Values{Name: "UFC 300", Type: "numbered", Status: "completed"}}, "test", time.Now())
			require.NoError(t, err)
			event.ID = "pre-upgrade-event"
			require.NoError(t, repo.transaction(ctx, func(tx *sql.Tx) error {
				if err := saveEvent(ctx, tx, event, 0); err != nil {
					return err
				}
				return loadComponentPreferences(ctx, tx, &event)
			}))
			events := snapshot(t, db, "ufc_events")
			require.NoError(t, repo.Migrate(ctx))
			require.NoError(t, repo.Migrate(ctx))
			var workspaceID string
			require.NoError(t, db.QueryRow("SELECT event_id FROM ufc_event_workspace WHERE event_id=$1", event.ID).Scan(&workspaceID))
			require.Equal(t, event.ID, workspaceID)
			require.Equal(t, events, snapshot(t, db, "ufc_events"))
			require.Equal(t, history, snapshot(t, db, "schema_migrations"))
			require.Equal(t, indexers, snapshot(t, db, "indexer"))
			j := acquisition.Job{EventRevision: event.Revision, SettingsRevision: event.Quality.SettingsRevision, ID: "manual-owned", EventID: event.ID, Component: "main_card", Title: "UFC.300.1080p", Source: "Saved source", Indexer: "saved-source", ClientID: 1, ClientName: "Saved SAB", Key: "stable-fingerprint", Endpoint: "saved-endpoint", Actor: "user:test", Size: 1234, Resolution: "1080p", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
			saved, created, err := repo.ReserveAcquisition(ctx, j)
			require.NoError(t, err)
			require.True(t, created)
			require.Positive(t, saved.ReleaseID)
			stale := j
			stale.ID = "stale-settings"
			stale.Key = "stale-settings-release"
			stale.Component = "early_prelims"
			stale.SettingsRevision--
			_, _, err = repo.ReserveAcquisition(ctx, stale)
			require.ErrorIs(t, err, catalogue.ErrRevisionConflict)
			j.ID = "other-request"
			duplicate, created, err := repo.ReserveAcquisition(ctx, j)
			require.NoError(t, err)
			require.False(t, created)
			require.Equal(t, saved.ID, duplicate.ID)
			j.Key = "different-release"
			_, created, err = repo.ReserveAcquisition(ctx, j)
			require.NoError(t, err)
			require.False(t, created)
			releases := upstream.NewReleaseRepo(zerolog.Nop(), appDB)
			release, err := releases.Get(ctx, &domain.GetReleaseRequest{Id: int(saved.ReleaseID)})
			require.NoError(t, err)
			require.Equal(t, saved.Title, release.TorrentName)
			require.Empty(t, release.DownloadURL)
			saved.State = "queued"
			saved.RemoteID = "sab-owned"
			require.NoError(t, repo.SaveAcquisition(ctx, saved))
			require.ErrorIs(t, repo.SaveAcquisition(ctx, saved), catalogue.ErrRevisionConflict)
			stats, err := releases.Stats(ctx)
			require.NoError(t, err)
			require.EqualValues(t, 1, stats.PushApprovedCount)
			jobs, err := repo.Acquisitions(ctx, true)
			require.NoError(t, err)
			require.Len(t, jobs, 1)
			require.Equal(t, "sab-owned", jobs[0].RemoteID)
			require.NoError(t, repo.DismissAcquisition(ctx, jobs[0].ID))
			visible, err := repo.Acquisitions(ctx, false)
			require.NoError(t, err)
			require.Empty(t, visible)
			jobs, err = repo.Acquisitions(ctx, true)
			require.NoError(t, err)
			require.Len(t, jobs, 1, "removed cards still reconcile without releasing duplicate protection")
			var originalProcessingSettings string
			require.NoError(t, db.QueryRowContext(ctx, "SELECT settings_json FROM ufc_processing_settings WHERE id=1").Scan(&originalProcessingSettings))
			handoffSettings, err := repo.ProcessingSettings(ctx)
			require.NoError(t, err)
			handoffSettings.AutoProcessCompleted = true
			handoffSettings.SABClientID = jobs[0].ClientID
			handoffJSON, err := json.Marshal(handoffSettings)
			require.NoError(t, err)
			_, err = db.ExecContext(ctx, "UPDATE ufc_processing_settings SET settings_json=$1 WHERE id=1", string(handoffJSON))
			require.NoError(t, err)
			jobs[0].State = "completed"
			jobs[0].Percent = 100
			jobs[0].Storage = "mock-complete-path"
			require.NoError(t, repo.SaveAcquisition(ctx, jobs[0]))
			pending, err := repo.PendingProcessingHandoffs(ctx)
			require.NoError(t, err)
			require.Len(t, pending, 1)
			require.Equal(t, saved.ID, pending[0].ID)
			_, err = db.ExecContext(ctx, "UPDATE ufc_processing_settings SET settings_json=$1 WHERE id=1", originalProcessingSettings)
			require.NoError(t, err)
			active, err := repo.Acquisitions(ctx, true)
			require.NoError(t, err)
			require.Empty(t, active)
			j.ID = "post-completion"
			_, created, err = repo.ReserveAcquisition(ctx, j)
			require.NoError(t, err)
			require.False(t, created)
			require.NoError(t, repo.ClearCatalogueEvents(ctx))
			jobs, err = repo.Acquisitions(ctx, false)
			require.NoError(t, err)
			require.Empty(t, jobs)
			var retainedState string
			require.NoError(t, db.QueryRow("SELECT state FROM ufc_acquisitions WHERE id=$1", saved.ID).Scan(&retainedState))
			require.Equal(t, "completed", retainedState)
			var count int
			require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM release_action_status WHERE action_id IS NULL AND filter_id IS NULL").Scan(&count))
			require.Equal(t, 1, count)
			before := snapshot(t, db, "release")
			_, err = db.Exec("ALTER TABLE release_action_status RENAME TO saved_actions")
			require.NoError(t, err)
			otherEvent, err := repo.CreateCatalogueEvent(ctx, catalogue.EventCreate{Values: catalogue.Values{Name: "Other", Type: "numbered", Status: "completed"}}, "test")
			require.NoError(t, err)
			j.EventRevision = otherEvent.Revision
			j.SettingsRevision = otherEvent.Quality.SettingsRevision
			j.ID = "rollback"
			j.EventID = otherEvent.ID
			j.Key = "rollback-key"
			_, _, err = repo.ReserveAcquisition(ctx, j)
			require.Error(t, err)
			require.Equal(t, before, snapshot(t, db, "release"))
			jobs, err = repo.Acquisitions(ctx, false)
			require.NoError(t, err)
			require.Empty(t, jobs)
			var retainedCount int
			require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM ufc_acquisitions").Scan(&retainedCount))
			require.Equal(t, 1, retainedCount, "failed projection must not leave a new reservation")
			_, err = db.Exec("ALTER TABLE saved_actions RENAME TO release_action_status")
			require.NoError(t, err)
			j.ID = "rejected-intent"
			j.EventID = otherEvent.ID
			j.Component = "prelims"
			j.Key = "retry-release"
			rejected, created, err := repo.ReserveAcquisition(ctx, j)
			require.NoError(t, err)
			require.True(t, created)
			rejected.State = "failed"
			require.NoError(t, repo.SaveAcquisition(ctx, rejected))
			duplicate, created, err = repo.ReserveAcquisition(ctx, j)
			require.NoError(t, err)
			require.False(t, created)
			duplicate.State = "queued"
			require.ErrorIs(t, repo.SaveAcquisition(ctx, duplicate), catalogue.ErrRevisionConflict)
			j.ID = "fresh-confirmed-selection"
			_, created, err = repo.ReserveAcquisition(ctx, j)
			require.NoError(t, err)
			require.True(t, created)
			t.Run("library", func(t *testing.T) { checkLibrary(t, repo) })
			t.Run("large library groups", func(t *testing.T) { checkLargeLibraryGroups(t, repo) })
			t.Run("manual workflow", func(t *testing.T) { checkManualWorkflow(t, repo) })
			t.Run("processing settings", func(t *testing.T) { checkProcessingSettings(t, repo) })
			t.Run("processing jobs", func(t *testing.T) {
				_, err := db.ExecContext(t.Context(), `INSERT INTO ufc_library_roots(id,path,enabled,state) VALUES('processing-fixture-root','fixture-library',TRUE,'available')`)
				require.NoError(t, err)
				job := processing.Job{ID: "processing-fixture-job", AcquisitionID: "manual-owned", EventID: event.ID, Component: "main_card", DestinationRoot: "fixture-library", State: "queued", CleanupRequested: true, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Files: []processing.JobFile{{Source: "source.mkv", Destination: "UFC 300 (2026)/UFC 300 (2026).mkv", Stage: "UFC 300 (2026)/.ufcm-fixture.part", Size: 24}}}
				stored, err := repo.EnqueueProcessingJob(t.Context(), job)
				require.NoError(t, err)
				finished, err := repo.FinishProcessingHandoff(t.Context(), job.AcquisitionID)
				require.NoError(t, err)
				require.True(t, finished)
				pending, err := repo.PendingProcessingHandoffs(t.Context())
				require.NoError(t, err)
				require.Empty(t, pending)
				require.Equal(t, "queued", stored.State)
				require.Len(t, stored.Files, 1)
				require.Equal(t, stored.Files[0].Destination, stored.Files[0].OriginalDestination)
				duplicate, err := repo.EnqueueProcessingJob(t.Context(), job)
				require.NoError(t, err)
				require.Equal(t, stored.ID, duplicate.ID)
				claimed, ok, err := repo.ClaimProcessingJob(t.Context())
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, stored.ID, claimed.ID)
				require.NoError(t, repo.SetProcessingFile(t.Context(), job.ID, "source.mkv", "staging", "", ""))
				require.NoError(t, repo.SetProcessingProgress(t.Context(), job.ID, "source.mkv", 12))
				require.NoError(t, repo.SetProcessingFile(t.Context(), job.ID, "source.mkv", "published", "hash", ""))
				require.NoError(t, repo.CompleteProcessingJob(t.Context(), claimed, 24, time.Now().UTC().Format(time.RFC3339Nano), "hash"))
				completed, err := repo.ProcessingJob(t.Context(), job.ID)
				require.NoError(t, err)
				require.Equal(t, "completed", completed.State)
				require.Equal(t, "verified", completed.Files[0].State)
				require.Equal(t, "pending", completed.CleanupState)
				cleanup, ok, err := repo.ClaimProcessingCleanup(t.Context())
				require.NoError(t, err)
				require.True(t, ok)
				require.Equal(t, job.ID, cleanup.ID)
				require.NoError(t, repo.FinishProcessingCleanup(t.Context(), job.ID, "needs_review", "fixture retained"))
				_, ok, err = repo.ClaimProcessingCleanup(t.Context())
				require.NoError(t, err)
				require.False(t, ok)
				require.NoError(t, repo.RetryProcessingCleanup(t.Context(), job.ID))
				_, ok, err = repo.ClaimProcessingCleanup(t.Context())
				require.NoError(t, err)
				require.True(t, ok)
				require.NoError(t, repo.FinishProcessingCleanup(t.Context(), job.ID, "done", ""))
				var inventory int
				require.NoError(t, db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM ufc_library_files WHERE event_id=$1 AND actor='processing'`, event.ID).Scan(&inventory))
				require.Equal(t, 1, inventory)
				multi := processing.Job{ID: "processing-multi-job", AcquisitionID: "fresh-confirmed-selection", EventID: otherEvent.ID, Component: "main_card", DestinationRoot: "fixture-library", State: "queued", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Files: []processing.JobFile{
					{Source: "a-main.mkv", Component: "main_card", Destination: "Multi/Main.mkv", Stage: "Multi/.ufcm-multi-1.part", Size: 30},
					{Source: "b-prelims.mkv", Component: "prelims", Destination: "Multi/Featurettes/Prelims.mkv", Stage: "Multi/Featurettes/.ufcm-multi-2.part", Size: 20},
				}}
				_, err = repo.EnqueueProcessingJob(t.Context(), multi)
				require.NoError(t, err)
				claimedMulti, ok, err := repo.ClaimProcessingJob(t.Context())
				require.NoError(t, err)
				require.True(t, ok)
				require.Len(t, claimedMulti.Files, 2)
				require.Equal(t, "prelims", claimedMulti.Files[1].Component)
				for index, file := range claimedMulti.Files {
					require.NoError(t, repo.SetProcessingFile(t.Context(), multi.ID, file.Source, "published", "hash", ""))
					one := claimedMulti
					one.Files = []processing.JobFile{file}
					require.NoError(t, repo.CompleteProcessingJob(t.Context(), one, file.Size, time.Now().UTC().Format(time.RFC3339Nano), "hash"))
					if index == 0 {
						partial, err := repo.ProcessingJob(t.Context(), multi.ID)
						require.NoError(t, err)
						require.Equal(t, "running", partial.State)
					}
				}
				completedMulti, err := repo.ProcessingJob(t.Context(), multi.ID)
				require.NoError(t, err)
				require.Equal(t, "completed", completedMulti.State)
				require.Equal(t, "verified", completedMulti.Files[0].State)
				require.Equal(t, "verified", completedMulti.Files[1].State)
				reviewJob := processing.Job{ID: "processing-review-job", AcquisitionID: "rejected-intent", EventID: otherEvent.ID, Component: "prelims", DestinationRoot: "fixture-library", State: "needs_review", Message: "Conflict requires review", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Files: []processing.JobFile{{Source: "prelims.mkv", Destination: "Other/Other - Prelims.mkv", Stage: "Other/.ufcm-review.part", Size: 24}}}
				_, err = repo.EnqueueProcessingJob(t.Context(), reviewJob)
				require.NoError(t, err)
				require.NoError(t, repo.SetProcessingDestination(t.Context(), reviewJob.ID, "Other/Other - Prelims (copy 2).mkv"))
				updated, err := repo.ProcessingJob(t.Context(), reviewJob.ID)
				require.NoError(t, err)
				require.Equal(t, "queued", updated.State)
				require.Equal(t, reviewJob.Files[0].Destination, updated.Files[0].OriginalDestination)
				require.Equal(t, "Other/Other - Prelims (copy 2).mkv", updated.Files[0].Destination)
				_, ok, err = repo.ClaimProcessingJob(t.Context())
				require.NoError(t, err)
				require.True(t, ok)
				require.NoError(t, repo.ReviewProcessingJob(t.Context(), reviewJob.ID, "Needs review"))
				require.NoError(t, repo.ResolveProcessingJob(t.Context(), reviewJob.ID, "skipped", "Skipped by user"))
				resolved, err := repo.ProcessingJob(t.Context(), reviewJob.ID)
				require.NoError(t, err)
				require.Equal(t, "skipped", resolved.Resolution)
			})
			t.Run("historical library aliases", func(t *testing.T) { checkHistoricalLibraryAliases(t, repo) })
		})
	}
}
