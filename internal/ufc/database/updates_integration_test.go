// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later
//go:build integration

package database

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	upstream "github.com/autobrr/autobrr/internal/database"
	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/updater"
	"github.com/autobrr/autobrr/pkg/migrator"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestUpdateUpgradeBothEngines(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := uint32(listener.Addr().(*net.TCPAddr).Port)
	require.NoError(t, listener.Close())
	cache, err := os.UserCacheDir()
	require.NoError(t, err)
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().Port(port).Version(embeddedpostgres.V17).Locale("C").Encoding("UTF8").Username("postgres").Password("updates-test-only").Database("updates").CachePath(filepath.Join(cache, "ufc-foundation", "archive")).BinariesPath(filepath.Join(cache, "ufc-foundation", "binaries")).RuntimePath(filepath.Join(t.TempDir(), "runtime")))
	require.NoError(t, pg.Start())
	t.Cleanup(func() { require.NoError(t, pg.Stop()) })
	for _, engine := range []string{"sqlite", "postgres"} {
		t.Run(engine, func(t *testing.T) {
			ctx := t.Context()
			dsn := filepath.Join(t.TempDir(), "updates.db")
			if engine == "postgres" {
				dsn = fmt.Sprintf("postgres://postgres:updates-test-only@127.0.0.1:%d/updates?sslmode=disable", port)
			}
			appDB, err := upstream.NewDB(&domain.Config{DatabaseType: engine, DatabaseDSN: dsn, DatabaseAutoMigrate: true}, zerolog.Nop())
			require.NoError(t, err)
			require.NoError(t, appDB.Open())
			t.Cleanup(func() { require.NoError(t, appDB.Close()) })
			repo := New(appDB.Handler, engine, zerolog.Nop())
			old := migrator.NewMigrate(appDB.Handler, migrator.WithEngine(engine), migrator.WithTableName("ufc_schema_migrations"), migrator.WithLegacyConversions(false), migrator.WithEmbedFS(migrations, "migrations/"+engine), migrator.WithLogger(zerolog.Nop()))
			for _, migration := range repo.migrator().GetUpTo("17_automatic_acquisition") {
				old.AddFileMigration(migration.File)
			}
			require.NoError(t, old.InitVersionTable())
			require.NoError(t, old.Migrate())
			tables := []string{"schema_migrations", "ufc_application_settings", "ufc_processing_settings", "ufc_events", "ufc_library_roots", "ufc_acquisitions"}
			before := map[string]string{}
			for _, table := range tables {
				before[table] = snapshot(t, appDB.Handler, table)
			}
			require.NoError(t, repo.Migrate(ctx))
			require.NoError(t, repo.Migrate(ctx))
			for _, table := range tables {
				require.Equal(t, before[table], snapshot(t, appDB.Handler, table), table)
			}
			state, err := repo.UpdateState(ctx)
			require.NoError(t, err)
			require.False(t, state.AutoInstall)
			require.Equal(t, "idle", state.Phase)
			stale := state
			state.ID = "signed-fixture"
			state.Version = "v1.0.1"
			state.Highwater = 2
			state.Phase = "verified"
			state.Envelope = updater.Envelope{Manifest: []byte("fixture"), Signature: []byte("test-signature")}
			state.Notice = &domain.NotificationPayload{Event: domain.NotificationEventAppUpdateAvailable, CurrentVersion: "1.0.0", NewVersion: state.Version, Subject: "UFC Manager Update Available", Message: "Signed update available"}
			saved, err := repo.SaveUpdateState(ctx, state)
			require.NoError(t, err)
			_, err = repo.SaveUpdateState(ctx, stale)
			require.ErrorIs(t, err, catalogue.ErrRevisionConflict)
			restarted, err := New(appDB.Handler, engine, zerolog.Nop()).UpdateState(ctx)
			require.NoError(t, err)
			require.Equal(t, saved, restarted)
			notices, err := repo.AutomationNotices(ctx, time.Now().Add(time.Minute))
			require.NoError(t, err)
			require.Len(t, notices, 1)
			var payload domain.NotificationPayload
			require.NoError(t, json.Unmarshal([]byte(notices[0].Message), &payload))
			require.Equal(t, "v1.0.1", payload.NewVersion)
			rollback := restarted
			rollback.Highwater = 1
			_, err = repo.SaveUpdateState(ctx, rollback)
			require.ErrorIs(t, err, catalogue.ErrRevisionConflict)
		})
	}
}
