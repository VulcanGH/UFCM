// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package updater

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallerMaintenanceExcludesLiveProcessesAndSurvivesRestart(t *testing.T) {
	root := filepath.Join(t.TempDir(), "runtime")
	profile := t.TempDir()
	require.NoError(t, os.Mkdir(root, 0700))
	running, err := RuntimeLock(profile)
	require.NoError(t, err)
	require.Error(t, BeginMaintenance(root, profile, "product-a"))
	running()
	require.NoError(t, BeginMaintenance(root, profile, "product-a"))
	require.Error(t, CheckMaintenance(root))
	require.Error(t, CheckMaintenance(profile))
	require.Error(t, BeginMaintenance(root, profile, "product-b"))
	require.Error(t, EndMaintenance(root, "product-b"))
	require.Error(t, CheckMaintenance(profile))
	require.NoError(t, BeginMaintenance(root, profile, "product-a"))
	require.NoError(t, EndMaintenance(root, "product-a"))
	require.NoError(t, CheckMaintenance(root))
	require.NoError(t, CheckMaintenance(profile))
	require.NoError(t, EndMaintenance(root, "product-a"))
}
