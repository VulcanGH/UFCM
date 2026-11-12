// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

//go:build foundation_e2e

package foundation_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/require"
)

func TestFoundationVariants(t *testing.T) {
	rootBytes, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}").Output()
	require.NoError(t, err)
	root := strings.TrimSpace(string(rootBytes))
	for _, variant := range []string{"autobrr", "ufc-manager"} {
		t.Run(variant, func(t *testing.T) {
			work := t.TempDir()
			binary := filepath.Join(work, variant)
			if runtime.GOOS == "windows" {
				binary += ".exe"
			}
			build := exec.Command("go", "build", "-ldflags", "-X github.com/autobrr/autobrr/internal/product.variant="+variant, "-o", binary, "./cmd/autobrr")
			build.Dir = root
			buildOutput, err := build.CombinedOutput()
			require.NoError(t, err, string(buildOutput))
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			port := listener.Addr().(*net.TCPAddr).Port
			require.NoError(t, listener.Close())
			config := filepath.Join(work, "config")
			require.NoError(t, os.Mkdir(config, 0700))
			require.NoError(t, os.WriteFile(filepath.Join(config, "config.toml"), []byte(fmt.Sprintf("port = %d\ncheckForUpdates = false\n", port)), 0600))
			process := exec.Command(binary, "--config", config)
			process.Dir = work
			process.Env = isolatedEnvironment()
			log, err := os.Create(filepath.Join(work, "server.log"))
			require.NoError(t, err)
			process.Stdout = log
			process.Stderr = log
			require.NoError(t, process.Start())
			t.Cleanup(func() { _ = process.Process.Kill(); _ = process.Wait(); _ = log.Close() })
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
			}, 15*time.Second, 100*time.Millisecond)
			anonymous := &http.Client{Timeout: 5 * time.Second}
			status := call(t, anonymous, http.MethodGet, base+"/api/ufc/status", nil, "")
			if variant == "ufc-manager" {
				require.Equal(t, 403, status.code)
			} else {
				require.Equal(t, 404, status.code)
			}
			credentials := map[string]any{"username": "foundation-user", "password": "Foundation-Test-Only-123!"}
			require.Equal(t, 200, call(t, client, http.MethodPost, base+"/api/auth/onboard", credentials, "").code)
			require.Equal(t, 204, call(t, client, http.MethodPost, base+"/api/auth/login", credentials, "").code)
			status = call(t, client, http.MethodGet, base+"/api/ufc/status", nil, "")
			if variant == "autobrr" {
				require.Equal(t, 404, status.code)
				require.Equal(t, 404, call(t, client, http.MethodGet, base+"/api/ufc/catalogue/status", nil, "").code)
				return
			}
			require.Equal(t, 200, status.code)
			require.Equal(t, false, status.data["automatic_acquisition_enabled"])
			require.Equal(t, false, status.data["processing_enabled"])
			catalogueStatus := call(t, client, http.MethodGet, base+"/api/ufc/catalogue/status", nil, "")
			require.Equal(t, 200, catalogueStatus.code)
			require.Equal(t, false, catalogueStatus.data["periodic_sync_enabled"])
			require.Equal(t, float64(0), catalogueStatus.data["events"])
			for _, scope := range []string{"ufc:read", "config:read"} {
				key := call(t, client, http.MethodPost, base+"/api/keys", map[string]any{"name": scope, "scopes": []string{scope}}, "")
				require.Equal(t, 201, key.code)
				token, ok := key.data["key"].(string)
				require.True(t, ok)
				result := call(t, anonymous, http.MethodGet, base+"/api/ufc/status", nil, token)
				catalogueResult := call(t, anonymous, http.MethodGet, base+"/api/ufc/catalogue/status", nil, token)
				if scope == "ufc:read" {
					require.Equal(t, 200, result.code)
					require.Equal(t, 200, catalogueResult.code)
				} else {
					require.Equal(t, 403, result.code)
					require.Equal(t, 403, catalogueResult.code)
				}
			}
			verifyWebhook(t, client, base, filepath.Join(work, "server.log"))
			pw, err := playwright.Run()
			require.NoError(t, err)
			defer pw.Stop()
			browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Headless: new(true)})
			require.NoError(t, err)
			defer browser.Close()
			context, err := browser.NewContext()
			require.NoError(t, err)
			defer context.Close()
			request, err := http.NewRequest("GET", base, nil)
			require.NoError(t, err)
			for _, cookie := range jar.Cookies(request.URL) {
				require.NoError(t, context.AddCookies([]playwright.OptionalCookie{{Name: cookie.Name, Value: cookie.Value, URL: new(base)}}))
			}
			page, err := context.NewPage()
			require.NoError(t, err)
			_, err = page.Goto(base + "/ufc/manager")
			require.NoError(t, err)
			headingErr := page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Settings", Exact: new(true)}).WaitFor()
			if headingErr != nil {
				text, _ := page.Locator("body").InnerText()
				t.Fatalf("UFC page %s: %v; body: %s", page.URL(), headingErr, text)
			}
			require.NoError(t, page.GetByText("Application ready", playwright.PageGetByTextOptions{Exact: new(true)}).WaitFor())
			statuses, err := page.Locator("dl dd").AllTextContents()
			require.NoError(t, err)
			require.Equal(t, []string{"Disabled", "Disabled"}, statuses)
			for _, path := range []string{"/filters", "/releases", "/settings", "/"} {
				_, err = page.Goto(base + path)
				require.NoError(t, err)
				require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "Settings", Exact: new(true)}).First().WaitFor())
				require.NotContains(t, page.URL(), "/login")
			}
			_, err = page.Goto(base + "/settings")
			require.NoError(t, err)
			require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "UFC", Exact: new(true)}).First().WaitFor())
		})
	}
}

type response struct {
	code int
	data map[string]any
	raw  string
}

func call(t *testing.T, client *http.Client, method, target string, data any, token string) response {
	t.Helper()
	var body io.Reader
	if data != nil {
		encoded, err := json.Marshal(data)
		require.NoError(t, err)
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, target, body)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Requested-With", "XMLHttpRequest")
	if token != "" {
		request.Header.Set("X-API-Token", token)
	}
	result, err := client.Do(request)
	require.NoError(t, err)
	defer result.Body.Close()
	raw, err := io.ReadAll(result.Body)
	require.NoError(t, err)
	value := response{code: result.StatusCode, raw: string(raw)}
	_ = json.Unmarshal(raw, &value.data)
	return value
}

func verifyWebhook(t *testing.T, client *http.Client, base, logPath string) {
	t.Helper()
	var deliveries atomic.Int32
	var failure atomic.Bool
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deliveries.Add(1)
		if failure.Load() {
			w.WriteHeader(400)
			fmt.Fprintf(w, "request rejected: %s", r.URL.String())
			return
		}
		w.WriteHeader(204)
	}))
	defer mock.Close()
	secret := mock.URL + "/api/webhooks/id/fake-foundation-webhook"
	values := map[string]any{"name": "foundation-hook", "type": "DISCORD", "enabled": true, "webhook": secret, "events": []string{"TEST"}}
	created := call(t, client, "POST", base+"/api/notification", values, "")
	require.Equal(t, 201, created.code, created.raw)
	require.NotContains(t, created.raw, "fake-foundation-webhook")
	id := int(created.data["id"].(float64))
	path := fmt.Sprintf("%s/api/notification/%d", base, id)
	values["id"] = id
	values["webhook"] = "<redacted>"
	require.Equal(t, 200, call(t, client, "PUT", path, values, "").code)
	testResult := call(t, client, "POST", base+"/api/notification/test", values, "")
	require.Equal(t, 204, testResult.code, testResult.raw)
	require.Greater(t, deliveries.Load(), int32(0))
	failure.Store(true)
	rejected := call(t, client, "POST", base+"/api/notification/test", values, "")
	require.Equal(t, 500, rejected.code)
	require.NotContains(t, rejected.raw, "fake-foundation-webhook")
	require.NotContains(t, rejected.raw, secret)
	failure.Store(false)
	logs, err := os.ReadFile(logPath)
	require.NoError(t, err)
	require.NotContains(t, string(logs), "fake-foundation-webhook")
	replacement := mock.URL + "/api/webhooks/id/fake-replacement-token"
	values["webhook"] = replacement
	updated := call(t, client, "PUT", path, values, "")
	require.Equal(t, 200, updated.code)
	require.NotContains(t, updated.raw, "fake-replacement-token")
	values["webhook"] = ""
	values["enabled"] = false
	cleared := call(t, client, "PUT", path, values, "")
	require.Equal(t, 200, cleared.code)
	require.Equal(t, "", cleared.data["webhook"])
}

func isolatedEnvironment() []string {
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if !strings.HasPrefix(upper, "AUTOBRR__") && !strings.HasPrefix(upper, "UFC_MANAGER__") {
			env = append(env, entry)
		}
	}
	return env
}
