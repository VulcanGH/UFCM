// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package migrator

import (
	"database/sql"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestIsolatedHistoryPreservesLegacyVersion(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec("PRAGMA user_version=99; CREATE TABLE schema_migrations(id INTEGER PRIMARY KEY, version TEXT); INSERT INTO schema_migrations VALUES(7,'upstream');")
	require.NoError(t, err)
	m := NewMigrate(db, WithEngine(EngineSQLite), WithTableName("ufc_schema_migrations"), WithLegacyConversions(false), WithLogger(zerolog.Nop()))
	m.Add(&Migration{Name: "foundation", RunTx: func(tx *sql.Tx) error { _, err := tx.Exec("CREATE TABLE ufc_settings(id INTEGER)"); return err }})
	require.NoError(t, m.Migrate())
	require.NoError(t, m.Migrate())
	var version int
	require.NoError(t, db.QueryRow("PRAGMA user_version").Scan(&version))
	require.Equal(t, 99, version)
	var name string
	require.NoError(t, db.QueryRow("SELECT version FROM schema_migrations WHERE id=7").Scan(&name))
	require.Equal(t, "upstream", name)
}

func TestMigrationCommitFailureReturned(t *testing.T) {
	for _, initial := range []bool{false, true} {
		t.Run(map[bool]string{false: "incremental", true: "initial"}[initial], func(t *testing.T) {
			db, err := sql.Open("sqlite", ":memory:")
			require.NoError(t, err)
			defer db.Close()
			_, err = db.Exec("PRAGMA foreign_keys=ON")
			require.NoError(t, err)
			schema := "CREATE TABLE parent(id INTEGER PRIMARY KEY); CREATE TABLE child(id INTEGER REFERENCES parent(id) DEFERRABLE INITIALLY DEFERRED); INSERT INTO child VALUES(1);"
			m := NewMigrate(db, WithEngine(EngineSQLite), WithLegacyConversions(false), WithLogger(zerolog.Nop()))
			if initial {
				m.initialSchema = schema
			}
			m.Add(&Migration{Name: "commit_failure", RunTx: func(tx *sql.Tx) error { _, err := tx.Exec(schema); return err }})
			require.Error(t, m.Migrate(), "a failed commit must never be reported as a successful migration")
		})
	}
}
