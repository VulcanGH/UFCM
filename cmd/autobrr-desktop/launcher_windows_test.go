// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/registry"
	"os"
	"path/filepath"
	"testing"
)

func TestLauncherLoginMigrationPreservesDisabledChoice(t *testing.T) {
	previous := desktopSettingsKey
	desktopSettingsKey = fmt.Sprintf(`Software\ufc-manager\launcher-test-%d`, os.Getpid())
	name := fmt.Sprintf("ufc-launcher-test-%d", os.Getpid())
	t.Cleanup(func() {
		_ = setStartOnLogin(name, "", false)
		_ = registry.DeleteKey(registry.CURRENT_USER, desktopSettingsKey)
		desktopSettingsKey = previous
	})
	directory := t.TempDir()
	profile := filepath.Join(directory, "profile")
	launcher := filepath.Join(directory, "ufc-manager-launcher.exe")
	old := filepath.Join(directory, "ufc-manager-desktop.exe")
	t.Setenv("UFC_MANAGER_LAUNCHER", launcher)
	key, _, err := registry.CreateKey(registry.CURRENT_USER, desktopSettingsKey, registry.SET_VALUE)
	require.NoError(t, err)
	require.NoError(t, key.SetStringValue("InstallDir", directory))
	require.NoError(t, key.SetDWordValue("LoginConfigured", 1))
	require.NoError(t, key.Close())
	require.NoError(t, setStartOnLogin(name, loginCommand(old, profile), true))
	require.NoError(t, initialiseInstalledLogin(launcher, name, loginCommand(launcher, profile)))
	on, err := startsOnLogin(name, loginCommand(launcher, profile))
	require.NoError(t, err)
	require.True(t, on)
	require.NoError(t, setStartOnLogin(name, "", false))
	require.NoError(t, initialiseInstalledLogin(launcher, name, loginCommand(launcher, profile)))
	on, err = startsOnLogin(name, loginCommand(launcher, profile))
	require.NoError(t, err)
	require.False(t, on)
}
