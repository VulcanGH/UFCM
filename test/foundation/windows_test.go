// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

//go:build foundation_e2e && windows

package foundation_test

import (
	"database/sql"
	"debug/pe"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestWindowsArtifacts(t *testing.T) {
	output, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}").Output()
	require.NoError(t, err)
	root := strings.TrimSpace(string(output))
	for _, slug := range []string{"autobrr", "ufc-manager"} {
		for _, tray := range []bool{false, true} {
			artifact := filepath.Join(t.TempDir(), slug+".exe")
			flags := "-X github.com/autobrr/autobrr/internal/product.variant=" + slug
			target := "./cmd/autobrr"
			if tray {
				flags = "-H=windowsgui " + flags
				target = "./cmd/autobrr-desktop"
			}
			build := exec.Command("go", "build", "-ldflags", flags, "-o", artifact, target)
			build.Dir = root
			data, err := build.CombinedOutput()
			require.NoError(t, err, string(data))
			if tray {
				image, err := pe.Open(artifact)
				require.NoError(t, err)
				require.Equal(t, uint16(2), image.OptionalHeader.(*pe.OptionalHeader64).Subsystem)
				require.NoError(t, image.Close())
			}
			for _, explicit := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/tray=%v/explicit=%v", slug, tray, explicit), func(t *testing.T) {
					work := t.TempDir()
					appdata := filepath.Join(work, "appdata")
					config := work
					if tray || slug == "ufc-manager" {
						config = filepath.Join(appdata, slug)
					}
					if explicit {
						config = filepath.Join(work, "explicit config")
					}
					require.NoError(t, os.MkdirAll(config, 0700))
					require.NoError(t, os.MkdirAll(appdata, 0700))
					require.NoError(t, os.WriteFile(filepath.Join(config, "config.toml"), []byte(fmt.Sprintf("checkForUpdates = %v\n", slug == "ufc-manager")), 0600))
					port := 7474
					if slug == "ufc-manager" {
						port = 7475
					}
					if explicit {
						port = 0
					}
					listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
					require.NoError(t, err, "default ports must be free for artifact acceptance")
					port = listener.Addr().(*net.TCPAddr).Port
					require.NoError(t, listener.Close())
					args := []string{}
					if explicit {
						args = append(args, "--config", config)
					}
					if !tray {
						args = append(args, "--pgo", filepath.Join(work, "profile.pprof"))
					}
					command := exec.Command(artifact, args...)
					command.Dir = work
					command.Env = append(isolatedEnvironment(), "APPDATA="+appdata)
					if explicit {
						prefix := "AUTOBRR__"
						if slug == "ufc-manager" {
							prefix = "UFC_MANAGER__"
						}
						command.Env = append(command.Env, fmt.Sprintf("%sPORT=%d", prefix, port))
					}
					command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
					log, err := os.Create(filepath.Join(work, "process.log"))
					require.NoError(t, err)
					command.Stdout = log
					command.Stderr = log
					require.NoError(t, command.Start())
					done := make(chan struct{})
					var waitErr error
					go func() { waitErr = command.Wait(); close(done) }()
					t.Cleanup(func() {
						select {
						case <-done:
						default:
							_ = command.Process.Kill()
							<-done
						}
						_ = log.Close()
					})
					base := fmt.Sprintf("http://127.0.0.1:%d", port)
					jar, err := cookiejar.New(nil)
					require.NoError(t, err)
					client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
					require.Eventually(t, func() bool {
						response, err := client.Get(base + "/api/healthz/readiness")
						if err != nil {
							return false
						}
						defer response.Body.Close()
						return response.StatusCode == 200
					}, 20*time.Second, 100*time.Millisecond)
					credentials := map[string]any{"username": "artifact-test", "password": "Fake-Artifact-Password-123!"}
					require.Equal(t, 200, call(t, client, "POST", base+"/api/auth/onboard", credentials, "").code)
					require.Equal(t, 204, call(t, client, "POST", base+"/api/auth/login", credentials, "").code)
					configuration := call(t, client, "GET", base+"/api/config", nil, "")
					require.Equal(t, 200, configuration.code)
					require.Equal(t, slug, configuration.data["product_slug"])
					require.Equal(t, filepath.Clean(config), filepath.Clean(configuration.data["config_dir"].(string)))
					require.Equal(t, float64(port), configuration.data["port"])
					if slug == "ufc-manager" {
						require.Equal(t, false, configuration.data["check_for_updates"])
						require.Equal(t, 204, call(t, client, "GET", base+"/api/updates/check", nil, "").code)
					}
					if tray {
						stopOwnedTray(t, command.Process.Pid)
					}
					select {
					case <-done:
						require.NoError(t, waitErr)
					case <-time.After(12 * time.Second):
						t.Fatal("artifact did not stop cleanly")
					}
					db, err := sql.Open("sqlite", filepath.Join(config, "autobrr.db"))
					require.NoError(t, err)
					defer db.Close()
					var count int
					require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name LIKE 'ufc_%'").Scan(&count))
					expected := 0
					if slug == "ufc-manager" {
						expected = 6
					}
					require.Equal(t, expected, count)
				})
			}
		}
	}
}

func stopOwnedTray(t *testing.T, pid int) {
	t.Helper()
	user := syscall.NewLazyDLL("user32.dll")
	found := false
	callback := syscall.NewCallback(func(window, parameter uintptr) uintptr {
		var owner uint32
		user.NewProc("GetWindowThreadProcessId").Call(window, uintptr(unsafe.Pointer(&owner)))
		if int(owner) != pid {
			return 1
		}
		class := make([]uint16, 256)
		user.NewProc("GetClassNameW").Call(window, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
		if syscall.UTF16ToString(class) == "autobrr-session" {
			found = true
			var result uintptr
			user.NewProc("SendMessageTimeoutW").Call(window, 0x16, 1, 0, 2, 10000, uintptr(unsafe.Pointer(&result)))
		}
		return 1
	})
	user.NewProc("EnumWindows").Call(callback, 0)
	require.True(t, found, "owned tray session-end window was not found")
}
