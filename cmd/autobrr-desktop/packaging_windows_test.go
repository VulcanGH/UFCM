// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/registry"
)

func TestInstalledStartupInitialisesOnlyOnceAtInstalledPath(t *testing.T) {
	previous := desktopSettingsKey
	desktopSettingsKey = fmt.Sprintf(`Software\ufc-manager\packaging-test-%d`, os.Getpid())
	name := fmt.Sprintf("ufc-package-test-%d", os.Getpid())
	t.Cleanup(func() {
		_ = setStartOnLogin(name, "", false)
		_ = registry.DeleteKey(registry.CURRENT_USER, desktopSettingsKey)
		desktopSettingsKey = previous
	})
	directory := t.TempDir()
	exe := filepath.Join(directory, "ufc-manager-desktop.exe")
	command := loginCommand(exe, filepath.Join(directory, "config"))
	key, _, err := registry.CreateKey(registry.CURRENT_USER, desktopSettingsKey, registry.SET_VALUE)
	require.NoError(t, err)
	require.NoError(t, key.SetStringValue("InstallDir", directory))
	require.NoError(t, key.Close())
	require.NoError(t, initialiseInstalledLogin(filepath.Join(t.TempDir(), "development.exe"), name, command))
	on, err := startsOnLogin(name, command)
	require.NoError(t, err)
	require.False(t, on)
	require.NoError(t, initialiseInstalledLogin(exe, name, command))
	on, err = startsOnLogin(name, command)
	require.NoError(t, err)
	require.True(t, on)
	require.NoError(t, setStartOnLogin(name, command, false))
	require.NoError(t, initialiseInstalledLogin(exe, name, command))
	on, err = startsOnLogin(name, command)
	require.NoError(t, err)
	require.False(t, on, "upgrade/restart must retain the disabled preference")
	require.NoError(t, cleanupInstalledLogin(name))
	require.NoError(t, initialiseInstalledLogin(exe, name, command))
	on, err = startsOnLogin(name, command)
	require.NoError(t, err)
	require.True(t, on, "fresh reinstall may initialise startup again")
}
