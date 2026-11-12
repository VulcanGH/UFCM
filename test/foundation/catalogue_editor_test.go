// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

//go:build foundation_e2e

package foundation_test

import (
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	ufcdatabase "github.com/autobrr/autobrr/internal/ufc/database"

	"github.com/mxschmitt/playwright-go"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestCatalogueEditorBrowser(t *testing.T) {
	rootBytes, err := exec.Command("go", "list", "-m", "-f", "{{.Dir}}").Output()
	require.NoError(t, err)
	root := strings.TrimSpace(string(rootBytes))
	work := t.TempDir()
	binary := filepath.Join(work, "ufc-manager")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-ldflags", "-X github.com/autobrr/autobrr/internal/product.variant=ufc-manager", "-o", binary, "./cmd/autobrr")
	build.Dir = root
	output, err := build.CombinedOutput()
	require.NoError(t, err, string(output))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	config := filepath.Join(work, "config")
	require.NoError(t, os.Mkdir(config, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(config, "config.toml"), []byte(fmt.Sprintf("host = '127.0.0.1'\nport = %d\ncheckForUpdates = false\n", port)), 0600))
	process := exec.Command(binary, "--config", config)
	process.Dir = work
	process.Env = isolatedEnvironment()
	log, err := os.Create(filepath.Join(work, "server.log"))
	require.NoError(t, err)
	process.Stdout, process.Stderr = log, log
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
	credentials := map[string]any{"username": "editor-test", "password": "Editor-Test-Only-123!"}
	require.Equal(t, 200, call(t, client, "POST", base+"/api/auth/onboard", credentials, "").code)
	require.Equal(t, 204, call(t, client, "POST", base+"/api/auth/login", credentials, "").code)
	db, err := sql.Open("sqlite", filepath.Join(config, "autobrr.db"))
	require.NoError(t, err)
	defer db.Close()
	repo := ufcdatabase.New(db, "sqlite", zerolog.Nop())
	date := "2027-01-30"
	fact := catalogue.Facts{PageID: 336, ListRevision: 1, ArticleRevision: 1, RetrievedAt: time.Now().UTC(), Title: "UFC 336", Name: "UFC 336: Source One vs. Source Two", Date: &date, Type: "numbered", Headliners: []string{"Source One", "Source Two"}, Status: "scheduled"}
	apply := func() {
		_, err := repo.ApplyCatalogue(t.Context(), &catalogue.Snapshot{ListRevision: 1, Discovered: 1, Facts: []catalogue.Facts{fact}})
		require.NoError(t, err)
	}
	apply()
	pw, err := playwright.Run()
	require.NoError(t, err)
	defer pw.Stop()
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Headless: new(true)})
	require.NoError(t, err)
	defer browser.Close()
	context, err := browser.NewContext(playwright.BrowserNewContextOptions{Viewport: &playwright.Size{Width: 1440, Height: 1100}})
	require.NoError(t, err)
	defer context.Close()
	request, err := http.NewRequest("GET", base, nil)
	require.NoError(t, err)
	for _, cookie := range jar.Cookies(request.URL) {
		require.NoError(t, context.AddCookies([]playwright.OptionalCookie{{Name: cookie.Name, Value: cookie.Value, URL: new(base)}}))
	}
	page, err := context.NewPage()
	require.NoError(t, err)
	page.SetDefaultTimeout(10000)
	_, err = page.Goto(base + "/ufc/events")
	require.NoError(t, err)
	button := func(name string) playwright.Locator {
		return page.GetByRole("button", playwright.PageGetByRoleOptions{Name: name, Exact: new(true)})
	}
	require.NoError(t, button("Edit "+fact.Name).Click())
	require.NoError(t, page.GetByLabel("Event name", playwright.PageGetByLabelOptions{Exact: new(true)}).Fill("Protected browser name"))
	require.NoError(t, page.GetByLabel("Event date", playwright.PageGetByLabelOptions{Exact: new(true)}).Fill(""))
	require.NoError(t, button("Save corrections").Click())
	require.NoError(t, page.GetByText("Event saved.", playwright.PageGetByTextOptions{Exact: new(true)}).WaitFor())
	fact.Name = "Latest source billing"
	fact.ArticleRevision++
	apply()
	_, err = page.Reload()
	require.NoError(t, err)
	require.NoError(t, button("Edit Protected browser name").Click())
	require.NoError(t, page.GetByText("Wikipedia suggestion: Latest source billing", playwright.PageGetByTextOptions{Exact: new(true)}).WaitFor())
	value, err := page.GetByLabel("Event name", playwright.PageGetByLabelOptions{Exact: new(true)}).InputValue()
	require.NoError(t, err)
	require.Equal(t, "Protected browser name", value)
	value, err = page.GetByLabel("Event date", playwright.PageGetByLabelOptions{Exact: new(true)}).InputValue()
	require.NoError(t, err)
	require.Empty(t, value)
	if screenshots := os.Getenv("UFC_EDITOR_SCREENSHOT_ROOT"); screenshots != "" {
		require.NoError(t, os.MkdirAll(screenshots, 0700))
		_, err = page.Screenshot(playwright.PageScreenshotOptions{Path: new(filepath.Join(screenshots, "editor-desktop.png")), FullPage: new(true)})
		require.NoError(t, err)
		require.NoError(t, page.SetViewportSize(390, 844))
		_, err = page.Screenshot(playwright.PageScreenshotOptions{Path: new(filepath.Join(screenshots, "editor-mobile.png")), FullPage: new(true)})
		require.NoError(t, err)
		require.NoError(t, page.SetViewportSize(1440, 1100))
	}
	require.NoError(t, button("Use default for Name").Click())
	require.NoError(t, button("Use default for Date").Click())
	require.NoError(t, button("Save corrections").Click())
	require.NoError(t, page.GetByText("Event saved.", playwright.PageGetByTextOptions{Exact: new(true)}).WaitFor())
	value, err = page.GetByLabel("Event date", playwright.PageGetByLabelOptions{Exact: new(true)}).InputValue()
	require.NoError(t, err)
	require.Equal(t, date, value)
	value, err = page.GetByLabel("Event name", playwright.PageGetByLabelOptions{Exact: new(true)}).InputValue()
	require.NoError(t, err)
	require.Equal(t, fact.Name, value)

	list, err := repo.CatalogueEvents(t.Context(), "2026-11-10")
	require.NoError(t, err)
	current, err := repo.LoadCatalogueEvent(t.Context(), list.Upcoming[0].ID)
	require.NoError(t, err)
	newer := "Newer external correction"
	_, err = repo.EditCatalogueEvent(t.Context(), current.ID, catalogue.EventEdit{Revision: current.Revision, Name: &catalogue.FieldEdit{Operation: "set", Value: &newer}}, "user:other-test")
	require.NoError(t, err)
	require.NoError(t, page.GetByLabel("Event name", playwright.PageGetByLabelOptions{Exact: new(true)}).Fill("Retained browser draft"))
	require.NoError(t, button("Save corrections").Click())
	require.NoError(t, page.GetByText("This event has a newer revision.", playwright.PageGetByTextOptions{Exact: new(false)}).WaitFor())
	value, err = page.GetByLabel("Event name", playwright.PageGetByLabelOptions{Exact: new(true)}).InputValue()
	require.NoError(t, err)
	require.Equal(t, "Retained browser draft", value)
	current, err = repo.LoadCatalogueEvent(t.Context(), current.ID)
	require.NoError(t, err)
	require.Equal(t, newer, current.Values.Name)
	require.NoError(t, button("Review latest revision").Click())
	require.NoError(t, button("Keep my draft against this revision").Click())
	require.NoError(t, button("Save corrections").Click())
	require.NoError(t, page.GetByText("Event saved.", playwright.PageGetByTextOptions{Exact: new(true)}).WaitFor())
	current, err = repo.LoadCatalogueEvent(t.Context(), current.ID)
	require.NoError(t, err)
	require.Equal(t, "Retained browser draft", current.Values.Name)
	_, err = page.GetByLabel("Prelims", playwright.PageGetByLabelOptions{Exact: new(true)}).SelectOption(playwright.SelectOptionValues{Values: &[]string{"off"}})
	require.NoError(t, err)
	_, err = page.GetByLabel("Embedded", playwright.PageGetByLabelOptions{Exact: new(true)}).SelectOption(playwright.SelectOptionValues{Values: &[]string{"on"}})
	require.NoError(t, err)
	require.NoError(t, button("Save corrections").Click())
	require.NoError(t, page.GetByText("Event saved.", playwright.PageGetByTextOptions{Exact: new(true)}).WaitFor())
	current, err = repo.LoadCatalogueEvent(t.Context(), current.ID)
	require.NoError(t, err)
	require.Equal(t, map[string]bool{"prelims": false, "embedded": true}, current.Components.Overrides)
	_, err = page.Goto(base + "/ufc/manager")
	require.NoError(t, err)
	require.NoError(t, page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Settings", Exact: new(true)}).WaitFor())
	require.Equal(t, base+"/ufc/settings", page.URL())
	require.NoError(t, page.Locator("#numbered-countdown").Click())
	require.NoError(t, page.GetByText("Explicit event overrides will be retained.", playwright.PageGetByTextOptions{Exact: new(true)}).WaitFor())
	require.NoError(t, button("Save settings").Click())
	require.NoError(t, page.GetByText("Component defaults saved.", playwright.PageGetByTextOptions{Exact: new(true)}).WaitFor())
	current, err = repo.LoadCatalogueEvent(t.Context(), current.ID)
	require.NoError(t, err)
	require.Contains(t, current.Components.Wanted, "countdown")
	require.Contains(t, current.Components.Wanted, "embedded")
	require.NotContains(t, current.Components.Wanted, "prelims")
	if screenshots := os.Getenv("UFC_EDITOR_SCREENSHOT_ROOT"); screenshots != "" {
		_, err = page.Screenshot(playwright.PageScreenshotOptions{Path: new(filepath.Join(screenshots, "component-settings-desktop.png")), FullPage: new(true)})
		require.NoError(t, err)
		require.NoError(t, page.SetViewportSize(390, 844))
		_, err = page.Screenshot(playwright.PageScreenshotOptions{Path: new(filepath.Join(screenshots, "component-settings-mobile.png")), FullPage: new(true)})
		require.NoError(t, err)
		require.NoError(t, page.SetViewportSize(1440, 1100))
	}
	_, err = page.Goto(base + "/ufc/events")
	require.NoError(t, err)
	require.NoError(t, button("Edit Retained browser draft").Click())
	choice, err := page.GetByLabel("Prelims", playwright.PageGetByLabelOptions{Exact: new(true)}).InputValue()
	require.NoError(t, err)
	require.Equal(t, "off", choice)
	_, err = page.GetByLabel("Prelims", playwright.PageGetByLabelOptions{Exact: new(true)}).SelectOption(playwright.SelectOptionValues{Values: &[]string{"inherit"}})
	require.NoError(t, err)
	require.NoError(t, button("Save corrections").Click())
	require.NoError(t, page.GetByText("Event saved.", playwright.PageGetByTextOptions{Exact: new(true)}).WaitFor())
	current, err = repo.LoadCatalogueEvent(t.Context(), current.ID)
	require.NoError(t, err)
	require.NotContains(t, current.Components.Overrides, "prelims")
	require.Contains(t, current.Components.Wanted, "prelims")
	_, err = page.Goto(base + "/settings")
	require.NoError(t, err)
	require.NoError(t, page.GetByText("Catalogue data and licences", playwright.PageGetByTextOptions{Exact: new(true)}).Click())
	require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "English Wikipedia contributors", Exact: new(true)}).WaitFor())
	require.NoError(t, page.GetByRole("link", playwright.PageGetByRoleOptions{Name: "CC BY-SA 4.0", Exact: new(true)}).WaitFor())
}
