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

	upstream "github.com/autobrr/autobrr/internal/database"
	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/pkg/migrator"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestFoundationBothEngines(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := uint32(listener.Addr().(*net.TCPAddr).Port)
	require.NoError(t, listener.Close())
	cache, err := os.UserCacheDir()
	require.NoError(t, err)
	runtimeRoot := t.TempDir()
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().Port(port).Version(embeddedpostgres.V17).Locale("C").Encoding("UTF8").Username("postgres").Password("foundation-test-only").Database("postgres").CachePath(filepath.Join(cache, "ufc-foundation", "archive")).BinariesPath(filepath.Join(cache, "ufc-foundation", "binaries")).RuntimePath(filepath.Join(runtimeRoot, "runtime")))
	require.NoError(t, pg.Start(), "PostgreSQL verification is required; do not skip an unavailable engine")
	t.Cleanup(func() { require.NoError(t, pg.Stop()) })
	adminDSN := fmt.Sprintf("postgres://postgres:foundation-test-only@127.0.0.1:%d/postgres?sslmode=disable", port)
	admin, err := sql.Open("postgres", adminDSN)
	require.NoError(t, err)
	defer admin.Close()
	for _, engine := range []string{"sqlite", "postgres"} {
		for _, scenario := range []string{"fresh", "foundation", "populated", "failure", "catalogue_failure"} {
			t.Run(engine+"/"+scenario, func(t *testing.T) {
				dsn := filepath.Join(t.TempDir(), "foundation.db")
				if engine == "postgres" {
					name := "foundation_" + scenario
					_, err := admin.Exec("CREATE DATABASE " + name)
					require.NoError(t, err)
					dsn = fmt.Sprintf("postgres://postgres:foundation-test-only@127.0.0.1:%d/%s?sslmode=disable", port, name)
				}
				cfg := &domain.Config{DatabaseType: engine, DatabaseDSN: dsn, DatabaseAutoMigrate: true}
				appDB, err := upstream.NewDB(cfg, zerolog.Nop())
				require.NoError(t, err)
				if scenario == "populated" {
					require.NoError(t, appDB.Open())
					defer appDB.Close()
				} else {
					handle, err := sql.Open(engine, dsn)
					require.NoError(t, err)
					appDB.Handler = handle
					defer handle.Close()
				}
				db := appDB.Handler
				var ufcTables int
				query := "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name LIKE 'ufc_%'"
				if engine == "postgres" {
					query = "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_name LIKE 'ufc_%'"
				}
				require.NoError(t, db.QueryRow(query).Scan(&ufcTables))
				require.Zero(t, ufcTables, "upstream startup must not create UFC schema")
				before := map[string]string{}
				if scenario == "populated" {
					for _, statement := range []string{
						"INSERT INTO users(id,username,password) VALUES(1001,'foundation-user','fake-hash')",
						"INSERT INTO api_key(id,name,key_hash) VALUES(1001,'foundation-key','fake-token-hash')",
						"INSERT INTO indexer(id,name,identifier) VALUES(1001,'foundation-indexer','foundation-indexer')",
						"INSERT INTO filter(id,name) VALUES(1001,'foundation-filter')",
						"INSERT INTO action(id,name,filter_id) VALUES(1001,'foundation-action',1001)",
						"INSERT INTO \"release\"(id,torrent_name,filter_id) VALUES(1001,'foundation-release',1001)",
					} {
						_, err := db.Exec(statement)
						require.NoError(t, err)
					}
					for _, table := range []string{"users", "api_key", "indexer", "filter", "action", "release", "schema_migrations"} {
						before[table] = snapshot(t, db, table)
					}
				}
				repo := New(db, engine, zerolog.Nop())
				if scenario == "foundation" || scenario == "populated" || scenario == "catalogue_failure" {
					m := migrator.NewMigrate(db, migrator.WithEngine(engine), migrator.WithTableName("ufc_schema_migrations"), migrator.WithLegacyConversions(false), migrator.WithEmbedFS(migrations, "migrations/"+engine), migrator.WithLogger(zerolog.Nop()))
					m.AddFileMigration("0_application_settings.sql")
					require.NoError(t, m.InitVersionTable())
					require.NoError(t, m.Migrate())
				}
				if scenario == "catalogue_failure" {
					history := snapshot(t, db, "ufc_schema_migrations")
					settings := snapshot(t, db, "ufc_application_settings")
					_, err := db.Exec("CREATE TABLE ufc_events(id INTEGER); INSERT INTO ufc_events VALUES(42)")
					require.NoError(t, err)
					require.Error(t, repo.Migrate(t.Context()))
					require.Equal(t, history, snapshot(t, db, "ufc_schema_migrations"))
					require.Equal(t, settings, snapshot(t, db, "ufc_application_settings"))
					require.Equal(t, "[42]", snapshot(t, db, "ufc_events"))
					var count int
					if engine == "sqlite" {
						err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE name='ufc_catalogue_settings'").Scan(&count)
					} else {
						err = db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_name='ufc_catalogue_settings'").Scan(&count)
					}
					require.NoError(t, err)
					require.Zero(t, count)
					return
				}
				if scenario == "failure" {
					_, err := db.Exec("CREATE TABLE ufc_application_settings(id INTEGER); INSERT INTO ufc_application_settings VALUES(42)")
					require.NoError(t, err)
					require.Error(t, repo.Migrate(t.Context()))
					var count, id int
					require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM ufc_schema_migrations").Scan(&count))
					require.Zero(t, count)
					require.NoError(t, db.QueryRow("SELECT id FROM ufc_application_settings").Scan(&id))
					require.Equal(t, 42, id)
					return
				}
				require.NoError(t, repo.Migrate(t.Context()))
				settings, err := repo.LoadApplicationSettings(t.Context())
				require.NoError(t, err)
				require.Equal(t, 1, settings.Revision)
				require.False(t, settings.AutomaticAcquisitionEnabled)
				require.False(t, settings.ProcessingEnabled)
				_, err = db.Exec("UPDATE ufc_application_settings SET revision=7 WHERE id=1")
				require.NoError(t, err)
				require.NoError(t, repo.Migrate(t.Context()))
				settings, err = repo.LoadApplicationSettings(t.Context())
				require.NoError(t, err)
				require.Equal(t, 7, settings.Revision)
				var count int
				require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM ufc_application_settings").Scan(&count))
				require.Equal(t, 1, count)
				require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM ufc_schema_migrations").Scan(&count))
				files, err := migrations.ReadDir("migrations/" + engine)
				require.NoError(t, err)
				require.Equal(t, len(files), count)
				var catalogueID int
				var catalogueVersion string
				require.NoError(t, db.QueryRow("SELECT id,version FROM ufc_schema_migrations WHERE id=2").Scan(&catalogueID, &catalogueVersion))
				require.Equal(t, 2, catalogueID)
				require.Equal(t, "2_event_catalogue", catalogueVersion)
				require.NoError(t, db.QueryRow("SELECT id,version FROM ufc_schema_migrations WHERE id=3").Scan(&catalogueID, &catalogueVersion))
				require.Equal(t, 3, catalogueID)
				require.Equal(t, "3_manual_events_sync", catalogueVersion)
				for _, change := range []string{"UPDATE ufc_schema_migrations SET version='unknown-schema' WHERE id=0", "UPDATE ufc_schema_migrations SET id=99 WHERE id=0"} {
					_, err = db.Exec(change)
					require.NoError(t, err)
					history := snapshot(t, db, "ufc_schema_migrations")
					require.Error(t, repo.Migrate(t.Context()))
					_, err = repo.LoadApplicationSettings(t.Context())
					require.Error(t, err, "read-only status must reject unknown history even without automatic migration")
					require.Equal(t, history, snapshot(t, db, "ufc_schema_migrations"))
					_, err = db.Exec("UPDATE ufc_schema_migrations SET id=0, version='0_application_settings' WHERE id IN (0,99)")
					require.NoError(t, err)
				}
				for table, expected := range before {
					require.Equal(t, expected, snapshot(t, db, table), "upstream data/history changed: %s", table)
				}
				if scenario == "fresh" || scenario == "populated" {
					t.Run("catalogue", func(t *testing.T) { testCatalogueRepository(t, repo) })
				}
			})
		}
	}
}

func snapshot(t *testing.T, db *sql.DB, table string) string {
	t.Helper()
	rows, err := db.Query("SELECT * FROM \"" + table + "\" ORDER BY id")
	require.NoError(t, err)
	defer rows.Close()
	columns, err := rows.Columns()
	require.NoError(t, err)
	result := ""
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		require.NoError(t, rows.Scan(pointers...))
		result += fmt.Sprint(values)
	}
	require.NoError(t, rows.Err())
	return result
}
