// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package config

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/product"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func TestProductConfiguration(t *testing.T) {
	for _, identity := range []product.Identity{product.Autobrr(), product.UFCManager()} {
		t.Run(identity.Slug(), func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			root := t.TempDir()
			cfg, err := LoadWithIdentity(root, identity)
			require.NoError(t, err)
			require.Equal(t, identity.Port(), cfg.Config.Port)
			require.Equal(t, root, cfg.Config.ConfigPath)
			require.Equal(t, identity.UpdatesEnabled(), cfg.Config.CheckForUpdates)
			require.NoError(t, os.WriteFile(filepath.Join(root, "config.toml"), []byte("port = 18765\ncheckForUpdates = true\n"), 0600))
			cfg, err = LoadWithIdentity(root, identity)
			require.NoError(t, err)
			require.Equal(t, 18765, cfg.Config.Port)
			require.Equal(t, identity.UpdatesEnabled(), cfg.Config.CheckForUpdates)
			t.Setenv(identity.EnvPrefix()+"PORT", "18766")
			cfg, err = LoadWithIdentity(root, identity)
			require.NoError(t, err)
			require.Equal(t, 18766, cfg.Config.Port)
		})
	}
}

func TestUFCConfigurationIsolated(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	root := t.TempDir()
	t.Setenv("APPDATA", root)
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("AUTOBRR__PORT", "18888")
	t.Setenv("AUTOBRR__CHECK_FOR_UPDATES", "true")
	t.Setenv("UFC_MANAGER__CHECK_FOR_UPDATES", "true")
	cfg, err := LoadWithIdentity("", product.UFCManager())
	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "ufc-manager"), cfg.Config.ConfigPath)
	require.Equal(t, 7475, cfg.Config.Port)
	require.False(t, cfg.Config.CheckForUpdates)
	require.NoFileExists(t, filepath.Join(root, "autobrr", "config.toml"))
}

func TestProductEnvironmentIntegerCompatibility(t *testing.T) {
	for _, identity := range []product.Identity{product.Autobrr(), product.UFCManager()} {
		cfg := &AppConfig{Config: &domain.Config{Product: identity}}
		for _, test := range []struct {
			value string
			want  int
		}{{"2147483647", 2147483647}, {"2147483648", 0}, {"-2147483649", 0}, {"invalid", 0}} {
			t.Setenv(identity.EnvPrefix()+"PORT", test.value)
			require.Equal(t, test.want, cfg.getEnvInt("PORT"))
		}
	}
}

func TestProductGeneratedConfiguration(t *testing.T) {
	t.Setenv("USERNAME", "ContainerAdministrator")
	for _, identity := range []product.Identity{product.Autobrr(), product.UFCManager()} {
		t.Run(identity.Slug(), func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			root := t.TempDir()
			cfg, err := LoadWithIdentity(root, identity)
			require.NoError(t, err)
			wantHost := "0.0.0.0"
			if identity.IsUFC() {
				wantHost = "127.0.0.1"
			}
			require.Equal(t, wantHost, cfg.Config.Host)
			file, err := os.ReadFile(filepath.Join(root, "config.toml"))
			require.NoError(t, err)
			require.Contains(t, string(file), "# Default: "+strconv.Itoa(identity.Port()))
			require.Contains(t, string(file), "# Environment variable: "+identity.EnvPrefix()+"OIDC_AUTO_REDIRECT")
		})
	}
}
