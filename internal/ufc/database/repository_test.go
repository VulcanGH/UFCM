// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package database

import (
	"context"
	"database/sql"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestSQLiteFoundation(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	repo := New(db, "sqlite", zerolog.Nop())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, repo.Migrate(ctx), context.Canceled)
	require.NoError(t, repo.Migrate(t.Context()))
	require.NoError(t, repo.Migrate(t.Context()))
	settings, err := repo.LoadApplicationSettings(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, settings.Revision)
	require.False(t, settings.ProcessingEnabled)
	require.False(t, settings.AutomaticAcquisitionEnabled)
	require.Error(t, New(db, "unknown", zerolog.Nop()).Migrate(t.Context()))
	require.NoError(t, db.Close())
	require.Error(t, repo.Migrate(t.Context()))
}
