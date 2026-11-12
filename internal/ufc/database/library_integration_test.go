// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

//go:build integration

package database

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/acquisition"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/library"
	"github.com/autobrr/autobrr/internal/ufc/matching"
	"github.com/stretchr/testify/require"
)

func checkHistoricalLibraryAliases(t *testing.T, repo *Repository) {
	date := "1993-11-12"
	facts := catalogue.Facts{PageID: 900001, ListRevision: 99, RetrievedAt: time.Now().UTC(), Title: "UFC 1", Name: "UFC 1", Type: "numbered", Date: &date, Status: "completed", Review: []string{"article_enrichment_pending"}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "1", Current: true, Verified: true}, {Namespace: "title", Value: "UFC 1: The Beginning", Current: true, Verified: true}}}
	_, err := repo.ApplyCatalogue(t.Context(), &catalogue.Snapshot{ListRevision: 99, Discovered: 1, Facts: []catalogue.Facts{facts}})
	require.NoError(t, err)
	events, err := repo.CatalogueMatchingEvents(t.Context())
	require.NoError(t, err)
	var expected string
	for _, event := range events {
		if event.Source.PageID == facts.PageID {
			expected = event.ID
			require.Contains(t, event.Aliases, catalogue.Alias{Namespace: "ufc.numbered", Value: "1", Current: true, Verified: false})
		}
	}
	require.NotEmpty(t, expected)
	file := library.File{RootID: "test", RootPath: "archive", Path: "UFC 1 The Beginning (1993)/UFC 1 The Beginning (1993).mp4", Size: 100}
	got := library.Identify(file, events, map[string]matching.Preview{})
	require.Equal(t, "confirmed", got.State, got.Reason)
	require.Equal(t, expected, got.EventID)
	require.NotEqual(t, "matched", matching.Match("UFC 1 The Beginning (1993)", events).Status)
}

func checkLargeLibraryGroups(t *testing.T, repo *Repository) {
	ctx := t.Context()
	settings, err := repo.LibrarySettings(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, settings.Roots)
	root := settings.Roots[0]
	t.Cleanup(func() {
		_, err := repo.db.ExecContext(context.Background(), "DELETE FROM ufc_library_files WHERE id LIKE 'large-%'")
		require.NoError(t, err)
		_, err = repo.db.ExecContext(context.Background(), "UPDATE ufc_library_roots SET state=$1,message=$2 WHERE id=$3", root.State, root.Message, root.ID)
		require.NoError(t, err)
	})
	files := make([]library.File, 0, 601)
	for i := 0; i < 601; i++ {
		files = append(files, library.File{ID: fmt.Sprintf("large-%d", i), Path: fmt.Sprintf("Event %d/media.mkv", i), Size: 1, Group: fmt.Sprintf("folder:large-%d", i), State: "review", Component: "unknown", Resolution: "unknown", Reason: "temporary metadata fixture"})
	}
	require.NoError(t, repo.BeginLibraryScan(ctx, "large-groups"))
	require.NoError(t, repo.CommitLibraryRoot(ctx, "large-groups", root, files, false))
	require.NoError(t, repo.FinishLibraryScan(ctx, "large-groups", "fixture complete", false))
	view, err := repo.LibraryView(ctx)
	require.NoError(t, err)
	seen := map[string]bool{}
	for _, group := range view.Groups {
		seen[group.ID] = true
	}
	for _, file := range files {
		require.True(t, seen[file.Group], file.Group)
	}
}

func checkLibrary(t *testing.T, repo *Repository) {
	ctx := t.Context()
	base := t.TempDir()
	rootPath := filepath.Join(base, "archive")
	folder := "UFC 173 Barao vs. Dillashaw (2014)"
	paths := []string{filepath.Join(folder, folder+".mkv"), filepath.Join(folder, "Featurettes", folder+" - Prelims.mkv"), filepath.Join(folder, "Featurettes", folder+" - Early Prelims.mkv")}
	for _, relative := range paths {
		full := filepath.Join(rootPath, relative)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0700))
		require.NoError(t, os.WriteFile(full, []byte("owned fixture"), 0600))
	}
	date := "2014-05-24"
	facts := catalogue.Facts{PageID: 173, ListRevision: 10, RetrievedAt: time.Now().UTC(), Title: "UFC 173", Name: "UFC 173: Barao vs. Dillashaw", Type: "numbered", Date: &date, Status: "unknown", Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "173", Current: true, Verified: true}}}
	merged, err := catalogue.Merge(catalogue.Event{}, facts)
	require.NoError(t, err)
	_, err = repo.ApplyCatalogue(ctx, &catalogue.Snapshot{ListRevision: 10, Discovered: 1, Facts: []catalogue.Facts{merged.Source}, ListRows: []catalogue.Facts{facts}})
	require.NoError(t, err)
	svc := library.New(repo)
	require.NoError(t, svc.Start(ctx))
	t.Cleanup(func() {
		stop, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, svc.Stop(stop))
	})
	settings, err := svc.Settings(ctx)
	require.NoError(t, err)
	initial := settings.Revision
	_, err = svc.SaveSettings(ctx, library.SettingsEdit{Revision: initial, Paths: []string{rootPath}})
	require.NoError(t, err)
	_, err = svc.SaveSettings(ctx, library.SettingsEdit{Revision: initial, Paths: []string{rootPath}})
	require.ErrorIs(t, err, catalogue.ErrRevisionConflict)
	scan := func() {
		t.Helper()
		_, err := svc.Scan(ctx)
		require.NoError(t, err)
		require.Eventually(t, func() bool { s, e := svc.Settings(ctx); return e == nil && !s.Running }, 10*time.Second, 20*time.Millisecond)
	}
	scan()
	view, err := svc.View(ctx)
	require.NoError(t, err)
	require.Len(t, view.Groups, 1)
	group := view.Groups[0]
	require.Equal(t, "present", group.Components["main_card"])
	require.Equal(t, "present", group.Components["early_prelims"])
	page, err := svc.Files(ctx, group.ID, 0)
	require.NoError(t, err)
	require.Len(t, page.Files, 3)
	var prelim, main library.File
	for _, f := range page.Files {
		if f.Component == "prelims" {
			prelim = f
		}
		if f.Component == "main_card" {
			main = f
		}
	}
	protected, err := svc.Edit(ctx, prelim.ID, library.FileEdit{Revision: prelim.Revision, Mode: "confirm", EventID: group.EventID, Component: "countdown", Resolution: "720p"}, "user:test")
	require.NoError(t, err)
	scan()
	current, err := repo.LibraryFile(ctx, prelim.ID)
	require.NoError(t, err)
	require.True(t, current.Manual)
	require.Equal(t, "countdown", current.Component)
	require.Equal(t, "720p", current.Resolution)
	_, err = svc.Edit(ctx, prelim.ID, library.FileEdit{Revision: protected.Revision, Mode: "ignore"}, "user:test")
	require.ErrorIs(t, err, catalogue.ErrRevisionConflict)
	require.NoError(t, os.WriteFile(filepath.Join(rootPath, filepath.FromSlash(prelim.Path)), []byte("changed owned fixture"), 0600))
	scan()
	current, err = repo.LibraryFile(ctx, prelim.ID)
	require.NoError(t, err)
	require.Equal(t, "review", current.State)
	require.Equal(t, "countdown", current.Component)
	current, err = svc.Edit(ctx, current.ID, library.FileEdit{Revision: current.Revision, Mode: "auto"}, "user:test")
	require.NoError(t, err)
	require.False(t, current.Manual)
	require.Equal(t, "prelims", current.Component)
	event, err := repo.LoadCatalogueEvent(ctx, group.EventID)
	require.NoError(t, err)
	name := "Protected archive title"
	event, err = repo.EditCatalogueEvent(ctx, event.ID, catalogue.EventEdit{Revision: event.Revision, Name: &catalogue.FieldEdit{Operation: "set", Value: &name}}, "test")
	require.NoError(t, err)
	changedSource := merged.Source
	changedSource.Name = "Changed source title"
	require.NoError(t, repo.AdoptLibraryEvents(ctx, &catalogue.Snapshot{Facts: []catalogue.Facts{changedSource}}))
	event, err = repo.LoadCatalogueEvent(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, name, event.Values.Name)
	j := acquisition.Job{ID: "library-block", EventID: event.ID, Component: "main_card", Title: "UFC.173.1080p", Source: "Saved source", ClientID: 1, ClientName: "Saved SAB", Key: "library-release", Endpoint: "saved", Actor: "test", Size: 10, Resolution: "1080p", EventRevision: event.Revision, SettingsRevision: event.Quality.SettingsRevision, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	_, _, err = repo.ReserveAcquisition(ctx, j)
	require.ErrorIs(t, err, catalogue.ErrInvalidEdit)
	offline := filepath.Join(base, "offline")
	require.NoError(t, os.Rename(rootPath, offline))
	scan()
	current, err = repo.LibraryFile(ctx, main.ID)
	require.NoError(t, err)
	require.True(t, current.Present)
	require.False(t, current.Available)
	view, err = svc.View(ctx)
	require.NoError(t, err)
	require.Equal(t, "unavailable", view.Groups[0].Components["main_card"])
	require.NoError(t, os.Rename(offline, rootPath))
	require.NoError(t, os.Remove(filepath.Join(rootPath, paths[2])))
	scan()
	view, err = svc.View(ctx)
	require.NoError(t, err)
	require.Equal(t, "missing", view.Groups[0].Components["early_prelims"])
	current, err = repo.LibraryFile(ctx, main.ID)
	require.NoError(t, err)
	_, err = svc.Edit(ctx, main.ID, library.FileEdit{Revision: current.Revision, Mode: "ignore"}, "test")
	require.NoError(t, err)
	_, created, err := repo.ReserveAcquisition(ctx, j)
	require.NoError(t, err)
	require.True(t, created)
	data, err := os.ReadFile(filepath.Join(rootPath, paths[0]))
	require.NoError(t, err)
	require.Equal(t, "owned fixture", string(data))
	current, err = repo.LibraryFile(ctx, prelim.ID)
	require.NoError(t, err)
	_, err = svc.Edit(ctx, current.ID, library.FileEdit{Revision: current.Revision, Mode: "confirm", EventID: event.ID, Component: "countdown", Resolution: "720p"}, "test")
	require.NoError(t, err)
	require.NoError(t, repo.ClearCatalogueEvents(ctx))
	current, err = repo.LibraryFile(ctx, prelim.ID)
	require.NoError(t, err)
	require.Equal(t, "confirmed", current.State)
	require.Equal(t, event.ID, current.EventID)
	listing, err := repo.CatalogueEvents(ctx, "2026-11-10")
	require.NoError(t, err)
	require.Empty(t, listing.Upcoming)
	require.Empty(t, listing.RecentCompleted)
	_, err = repo.ApplyCatalogue(ctx, &catalogue.Snapshot{ListRevision: 11, Discovered: 1, Facts: []catalogue.Facts{merged.Source}})
	require.NoError(t, err)
	eventAgain, err := repo.LoadCatalogueEvent(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, name, eventAgain.Values.Name)
	scan()
	current, err = repo.LibraryFile(ctx, prelim.ID)
	require.NoError(t, err)
	require.Equal(t, "confirmed", current.State)
	require.Equal(t, "countdown", current.Component)
	require.Equal(t, event.ID, current.DetectedEvent)
	var sourceRows string
	require.NoError(t, repo.db.QueryRow("SELECT rows_json FROM ufc_catalogue_source_list WHERE id=1").Scan(&sourceRows))
	require.Contains(t, sourceRows, "UFC 173")
	reset, err := svc.Edit(ctx, current.ID, library.FileEdit{Revision: current.Revision, Mode: "auto"}, "test")
	require.NoError(t, err)
	require.Equal(t, "confirmed", reset.State)
	require.Equal(t, current.DetectedEvent, reset.EventID)
	require.Equal(t, "prelims", reset.Component)
	futureDate := "2027-01-01"
	future := catalogue.Facts{PageID: 998, Title: "UFC 998", Name: "UFC 998", Type: "numbered", Status: "scheduled", Date: &futureDate, ListRevision: 12, RetrievedAt: time.Now().UTC(), Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "998", Current: true, Verified: true}}}
	_, err = repo.ApplyCatalogue(ctx, &catalogue.Snapshot{ListRevision: 12, Discovered: 1, Facts: []catalogue.Facts{future}})
	require.NoError(t, err)
	var futureID string
	require.NoError(t, repo.db.QueryRow("SELECT id FROM ufc_events WHERE source_page_id=998").Scan(&futureID))
	future.Status, future.OverallSequence = "completed", "798"
	futureDate = "2026-11-01"
	_, err = repo.ApplyCatalogue(ctx, &catalogue.Snapshot{ListRevision: 13, Discovered: 1, Facts: []catalogue.Facts{future}})
	require.NoError(t, err)
	completed, err := repo.LoadCatalogueEvent(ctx, futureID)
	require.NoError(t, err)
	require.Equal(t, "completed", completed.Values.Status)
	require.Equal(t, "798", completed.Source.OverallSequence)
	listing, err = repo.CatalogueEvents(ctx, "2026-11-10")
	require.NoError(t, err)
	require.Len(t, listing.RecentCompleted, 1)
	require.Equal(t, futureID, listing.RecentCompleted[0].ID)
	catalogueBefore := snapshot(t, repo.db, "ufc_events")
	settings, err = svc.Settings(ctx)
	require.NoError(t, err)
	require.NoError(t, repo.BeginLibraryScan(ctx, "clear-race"))
	runningSettings, err := svc.Settings(ctx)
	require.NoError(t, err)
	require.ErrorIs(t, svc.Clear(ctx, runningSettings.Revision), catalogue.ErrRevisionConflict)
	require.NoError(t, repo.FinishLibraryScan(ctx, "clear-race", "Fixture finished", false))
	require.ErrorIs(t, svc.Clear(ctx, settings.Revision), catalogue.ErrRevisionConflict)
	settings, err = svc.Settings(ctx)
	require.NoError(t, err)
	require.NoError(t, svc.Clear(ctx, settings.Revision))
	view, err = svc.View(ctx)
	require.NoError(t, err)
	require.Empty(t, view.Groups)
	require.Len(t, view.Settings.Roots, 1)
	require.Equal(t, rootPath, view.Settings.Roots[0].Path)
	require.Equal(t, "unscanned", view.Settings.Roots[0].State)
	require.Nil(t, view.Settings.LastScan)
	require.Equal(t, catalogueBefore, snapshot(t, repo.db, "ufc_events"))
	data, err = os.ReadFile(filepath.Join(rootPath, paths[0]))
	require.NoError(t, err)
	require.Equal(t, "owned fixture", string(data))
	scan()
	view, err = svc.View(ctx)
	require.NoError(t, err)
	require.Len(t, view.Groups, 1)
	require.Equal(t, "present", view.Groups[0].Components["main_card"])
	night, err := repo.CreateCatalogueEvent(ctx, catalogue.EventCreate{Values: catalogue.Values{Name: "Fight Night fixture", Type: "fight_night", Status: "completed"}}, "test")
	require.NoError(t, err)
	view, err = svc.View(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"main_card", "prelims"}, view.Wanted[night.ID])
	night, err = repo.EditCatalogueEvent(ctx, night.ID, catalogue.EventEdit{Revision: night.Revision, Components: map[string]string{"early_prelims": "on"}}, "test")
	require.NoError(t, err)
	view, err = svc.View(ctx)
	require.NoError(t, err)
	require.Contains(t, view.Wanted[night.ID], "early_prelims")
	night, err = repo.EditCatalogueEvent(ctx, night.ID, catalogue.EventEdit{Revision: night.Revision, Components: map[string]string{"early_prelims": "inherit"}}, "test")
	require.NoError(t, err)
	prefs, err := repo.CataloguePreferences(ctx)
	require.NoError(t, err)
	prefs.Templates["fight_night"] = []string{"main_card"}
	_, err = repo.SaveCataloguePreferences(ctx, catalogue.SettingsEdit{Revision: prefs.Revision, Templates: prefs.Templates})
	require.NoError(t, err)
	view, err = svc.View(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"main_card"}, view.Wanted[night.ID])
	beforeReset := map[string]string{}
	for _, table := range []string{"ufc_library_files", "ufc_library_roots", "ufc_acquisitions", "release", "release_action_status"} {
		beforeReset[table] = snapshot(t, repo.db, table)
	}
	oldEvent, err := repo.LoadCatalogueEvent(ctx, event.ID)
	require.NoError(t, err)
	prefs, err = repo.CataloguePreferences(ctx)
	require.NoError(t, err)
	enabled := true
	_, err = repo.SaveCataloguePreferences(ctx, catalogue.SettingsEdit{Revision: prefs.Revision, Templates: prefs.Templates, PeriodicSyncEnabled: &enabled})
	require.NoError(t, err)
	require.NoError(t, repo.ResetCatalogueEvents(ctx))
	for _, table := range []string{"ufc_events", "ufc_event_aliases", "ufc_event_workspace", "ufc_event_quality_overrides", "ufc_event_component_overrides", "ufc_catalogue_source_list"} {
		var count int
		require.NoError(t, repo.db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&count))
		require.Zero(t, count, table)
	}
	for table, before := range beforeReset {
		require.Equal(t, before, snapshot(t, repo.db, table), table)
	}
	prefs, err = repo.CataloguePreferences(ctx)
	require.NoError(t, err)
	require.False(t, prefs.PeriodicSyncEnabled)
	require.Nil(t, prefs.NextDue)
	_, err = repo.ApplyCatalogue(ctx, &catalogue.Snapshot{ListRevision: 14, Discovered: 1, Facts: []catalogue.Facts{merged.Source}})
	require.NoError(t, err)
	restored, err := repo.LoadCatalogueEvent(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, merged.Source.Name, restored.Values.Name)
	require.Greater(t, restored.Revision, oldEvent.Revision)
	_, err = repo.EditCatalogueEvent(ctx, event.ID, catalogue.EventEdit{Revision: oldEvent.Revision, Name: &catalogue.FieldEdit{Operation: "set", Value: &name}}, "stale")
	require.ErrorIs(t, err, catalogue.ErrRevisionConflict)
	view, err = svc.View(ctx)
	require.NoError(t, err)
	require.Equal(t, event.ID, view.Groups[0].EventID)
	data, err = os.ReadFile(filepath.Join(rootPath, paths[0]))
	require.NoError(t, err)
	require.Equal(t, "owned fixture", string(data))
}
