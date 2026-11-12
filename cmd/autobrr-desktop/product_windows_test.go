// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

//go:build windows

package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/autobrr/autobrr/internal/product"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows/registry"
)

func TestProductRegistryIsolation(t *testing.T) {
	identity, err := product.Selected()
	require.NoError(t, err)
	require.Equal(t, identity.RegistryKey(), desktopSettingsKey)
	opposite := product.UFCManager()
	if identity.IsUFC() {
		opposite = product.Autobrr()
	}
	name := fmt.Sprintf("foundation-isolation-test-%d", os.Getpid())
	key, _, err := registry.CreateKey(registry.CURRENT_USER, opposite.RegistryKey(), registry.SET_VALUE|registry.QUERY_VALUE)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = key.DeleteValue(name)
		_ = key.Close()
		if selected, err := registry.OpenKey(registry.CURRENT_USER, desktopSettingsKey, registry.SET_VALUE); err == nil {
			_ = selected.DeleteValue(name)
			_ = selected.Close()
		}
	})
	require.NoError(t, key.SetDWordValue(name, 0))
	require.NoError(t, setDesktopSetting(name, true))
	enabled, err := desktopSetting(name)
	require.NoError(t, err)
	require.True(t, enabled)
	value, _, err := key.GetIntegerValue(name)
	require.NoError(t, err)
	require.Zero(t, value, "writing one product's settings changed the other namespace")
}

func TestUFCUpdaterDisabled(t *testing.T) {
	identity, err := product.Selected()
	require.NoError(t, err)
	if !identity.IsUFC() {
		t.Skip("requires the UFC product linker flag")
	}
	updater := newSelfUpdater("unused-foundation-test.exe")
	err = updater.install(t.Context(), nil)
	require.EqualError(t, err, "updates are disabled for this product")
}
