// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package cli

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSQLiteBackupDoesNotMigrateOrReplace(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source #1.db")
	destination := filepath.Join(t.TempDir(), "backup.db")
	db, err := sql.Open("sqlite", source)
	require.NoError(t, err)
	_, err = db.Exec("CREATE TABLE fixture(value TEXT); INSERT INTO fixture VALUES('preserve'); PRAGMA user_version=17")
	require.NoError(t, err)
	require.NoError(t, db.Close())
	before, err := os.ReadFile(source)
	require.NoError(t, err)
	require.NoError(t, backupSQLite(t.Context(), source, destination))
	after, err := os.ReadFile(source)
	require.NoError(t, err)
	require.Equal(t, before, after)
	copyDB, err := sql.Open("sqlite", destination)
	require.NoError(t, err)
	defer copyDB.Close()
	var value string
	require.NoError(t, copyDB.QueryRow("SELECT value FROM fixture").Scan(&value))
	require.Equal(t, "preserve", value)
	var version int
	require.NoError(t, copyDB.QueryRow("PRAGMA user_version").Scan(&version))
	require.Equal(t, 17, version)
	backup, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.Error(t, backupSQLite(t.Context(), source, destination))
	kept, err := os.ReadFile(destination)
	require.NoError(t, err)
	require.Equal(t, backup, kept)
	require.Error(t, backupSQLite(t.Context(), source, source))
	require.Error(t, backupSQLite(t.Context(), filepath.Join(t.TempDir(), "missing.db"), filepath.Join(t.TempDir(), "absent.db")))
}
