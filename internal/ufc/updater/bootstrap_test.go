// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package updater

import (
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLauncherChild(t *testing.T) {
	if os.Getenv("UFC_TEST_CHILD") != "1" {
		return
	}
	workingDirectory, err := os.Getwd()
	if err != nil || workingDirectory != os.Getenv("UFC_TEST_WORKDIR") || os.Getenv("UFC_MANAGER__PORT") != "19888" {
		os.Exit(10)
	}
	version := os.Getenv("UFC_TEST_VERSION")
	SchemaDigest = hash([]byte("fixture-schema"))
	profile := os.Getenv("UFC_TEST_PROFILE")
	mode := os.Getenv("UFC_TEST_MODE")
	dbPath := filepath.Join(profile, "test.db")
	if strings.HasPrefix(version, "v1.0.1") && strings.HasPrefix(mode, "fail") {
		if mode == "fail-schema" {
			db, err := sql.Open("sqlite", dbPath)
			if err != nil {
				os.Exit(9)
			}
			if _, err = db.Exec("ALTER TABLE fixture ADD COLUMN changed TEXT"); err != nil {
				os.Exit(9)
			}
			db.Close()
		}
		os.Exit(3)
	}
	if err := HostReady(context.Background(), Host{Version: version, Config: profile, Driver: "sqlite", Database: dbPath}); err != nil {
		os.Exit(4)
	}
	file, err := os.OpenFile(filepath.Join(profile, "activated.txt"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(5)
	}
	_, _ = file.WriteString(version + "\n")
	file.Close()
	os.Exit(0)
}

func TestLauncherTrialActivationFallbackAndSchemaRecovery(t *testing.T) {
	t.Setenv("UFC_MANAGER__PORT", "19888")
	workingDirectory, err := os.Getwd()
	require.NoError(t, err)
	t.Setenv("UFC_TEST_WORKDIR", workingDirectory)
	for _, mode := range []string{"success", "fail-before", "fail-schema", "backup-failure", "obsolete-request"} {
		t.Run(mode, func(t *testing.T) {
			base := t.TempDir()
			install := filepath.Join(base, "install")
			seedDir := filepath.Join(install, "seed")
			profile := filepath.Join(base, "profile")
			root := filepath.Join(base, "runtime")
			require.NoError(t, os.MkdirAll(seedDir, 0700))
			require.NoError(t, os.MkdirAll(profile, 0700))
			require.NoError(t, os.MkdirAll(root, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(profile, "config.toml"), []byte("# isolated test profile\n"), 0600))
			db, err := sql.Open("sqlite", filepath.Join(profile, "test.db"))
			require.NoError(t, err)
			_, err = db.Exec(`CREATE TABLE fixture(value TEXT);INSERT INTO fixture VALUES('retained');CREATE TABLE schema_migrations(id INTEGER,version TEXT);INSERT INTO schema_migrations VALUES(1,'baseline');CREATE TABLE ufc_schema_migrations(id INTEGER,version TEXT);INSERT INTO ufc_schema_migrations VALUES(1,'baseline')`)
			require.NoError(t, err)
			require.NoError(t, db.Close())
			files := []File{}
			seedVersion := "1.0.0"
			if mode == "obsolete-request" {
				seedVersion = "2.0.0"
			}
			for _, name := range []string{"ufc-manager-desktop.exe", "ufc-manager.exe"} {
				data := []byte("installer-baseline")
				require.NoError(t, os.WriteFile(filepath.Join(seedDir, name), data, 0700))
				files = append(files, File{Name: name, Size: int64(len(data)), SHA256: hash(data)})
			}
			require.NoError(t, WriteJSON(filepath.Join(seedDir, "seed.json"), Seed{Version: seedVersion, Files: files}))
			client, envelope, _, _, _ := fixture(t, "")
			_, err = client.Stage(t.Context(), filepath.Join(root, "versions"), envelope, 0)
			require.NoError(t, err)
			require.NoError(t, WriteJSON(filepath.Join(root, "request.json"), Request{Envelope: envelope, Database: filepath.Join(profile, "test.db"), Config: profile}))
			if mode == "backup-failure" {
				require.NoError(t, os.MkdirAll(filepath.Join(root, "backups"), 0700))
				require.NoError(t, os.WriteFile(filepath.Join(root, "backups", ID(envelope)), []byte("blocked backup directory"), 0600))
			}
			launcher := Launcher{Root: root, Install: install, Config: profile, Executable: "fixture-launcher", Trust: client.Trust, StartupTimeout: 5 * time.Second, Command: func(path string, args ...string) *exec.Cmd {
				version := seedVersion
				if filepath.Dir(path) != seedDir {
					version = "v1.0.1"
				}
				cmd := exec.Command(os.Args[0], "-test.run=^TestLauncherChild$")
				cmd.Env = []string{"UFC_TEST_CHILD=1", "UFC_TEST_VERSION=" + version, "UFC_TEST_PROFILE=" + profile, "UFC_TEST_MODE=" + mode}
				return cmd
			}}
			err = launcher.Run(t.Context())
			if mode == "fail-schema" {
				require.ErrorContains(t, err, "schema")
			} else {
				require.NoError(t, err)
			}
			var state Bootstrap
			require.NoError(t, ReadJSON(filepath.Join(root, "bootstrap-state.json"), &state))
			activated, readErr := os.ReadFile(filepath.Join(profile, "activated.txt"))
			require.NoError(t, readErr)
			switch mode {
			case "obsolete-request":
				require.Equal(t, "seed", state.Active)
				require.Equal(t, "2.0.0\n", string(activated))
				require.NoFileExists(t, filepath.Join(root, "request.json"))
			case "backup-failure":
				require.Equal(t, "seed", state.Active)
				require.Equal(t, ID(envelope), state.DeferredID)
				require.Empty(t, state.FailedID)
				require.Equal(t, "1.0.0\n1.0.0\n", string(activated))
			case "success":
				require.Equal(t, ID(envelope), state.Active)
				require.Equal(t, "seed", state.Previous)
				require.Equal(t, "1.0.0\nv1.0.1\n", string(activated))
			case "fail-before":
				require.Equal(t, "seed", state.Active)
				require.Equal(t, ID(envelope), state.FailedID)
				require.Equal(t, "1.0.0\n1.0.0\n", string(activated))
			case "fail-schema":
				require.Equal(t, "trial", state.Phase)
				require.Equal(t, "1.0.0\n", string(activated))
				require.ErrorContains(t, launcher.Run(t.Context()), "schema")
			}
			if mode != "backup-failure" && mode != "obsolete-request" {
				require.FileExists(t, filepath.Join(root, "backups", state.Backup, "database.db"))
				require.FileExists(t, filepath.Join(root, "backups", state.Backup, "config.toml"))
			}
		})
	}
}

func TestRuntimeLockSurvivesSeparateLauncherLockRelease(t *testing.T) {
	root := t.TempDir()
	parent, err := Lock(root)
	require.NoError(t, err)
	child, err := RuntimeLock(root)
	require.NoError(t, err)
	parent()
	defer child()
	_, err = RuntimeLock(root)
	require.Error(t, err)
}
