// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

//go:build integration

package database

import (
	"testing"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/stretchr/testify/require"
)

func checkProcessingSettings(t *testing.T, repo *Repository) {
	ctx := t.Context()
	defaults, err := repo.ProcessingSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), defaults.Revision)
	require.Equal(t, "review", defaults.ConflictPolicy)
	require.False(t, defaults.CleanupSABDownload)
	edit := *defaults
	edit.ComponentFolder = "Extras"
	saved, err := repo.SaveProcessingSettings(ctx, edit)
	require.NoError(t, err)
	require.Equal(t, int64(2), saved.Revision)
	_, err = repo.SaveProcessingSettings(ctx, edit)
	require.ErrorIs(t, err, catalogue.ErrRevisionConflict)
	read, err := repo.ProcessingSettings(ctx)
	require.NoError(t, err)
	require.Equal(t, "Extras", read.ComponentFolder)
	require.Equal(t, int64(2), read.Revision)
}
