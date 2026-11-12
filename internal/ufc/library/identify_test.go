// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/matching"
	"github.com/stretchr/testify/require"
)

func TestLargeLibraryMatchesPastEnrichmentBoundary(t *testing.T) {
	root := Root{ID: "large", Path: filepath.Join(t.TempDir(), "archive")}
	date := "2025-03-15"
	events := make([]catalogue.Event, 0, 601)
	for i := 0; i < 601; i++ {
		name := fmt.Sprintf("UFC Fight Night: Fighter%d vs. Opponent%d 2", i, i)
		folder := fmt.Sprintf("UFC Fight Night %d Fighter%d vs. Opponent%d 2 (2025)", i+1, i, i)
		nameOnDisk := strings.TrimSuffix(folder, " (2025)") + " - Prelims.mkv"
		full := filepath.Join(root.Path, folder, "Featurettes", nameOnDisk)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0700))
		require.NoError(t, os.WriteFile(full, []byte("temporary fixture"), 0600))
		e := catalogue.Event{ID: fmt.Sprint(i), Values: catalogue.Values{Name: name, Type: "fight_night", Date: &date, Status: "unknown"}}
		if i >= 56 {
			e.Review = []string{"article_enrichment_pending"}
		}
		events = append(events, e)
	}
	files, complete := walkRoot(t.Context(), root, 100000)
	require.True(t, complete)
	require.Len(t, files, 601)
	cache := map[string]matching.Preview{}
	groups := map[string]bool{}
	for _, file := range files {
		f := Identify(file, events, cache)
		require.Equal(t, "confirmed", f.State, "%s: %s", f.Path, f.Reason)
		require.Equal(t, "prelims", f.Component)
		groups[f.Group] = true
	}
	require.Len(t, groups, 601)
}

func TestPendingHistoricalFightNightUsesFolderYearWithoutTrustingNumber(t *testing.T) {
	date := "2025-03-15"
	event := catalogue.Event{ID: "vettori-dolidze", Values: catalogue.Values{Name: "UFC Fight Night: Vettori vs. Dolidze 2", Type: "fight_night", Date: &date, Status: "unknown"}, Review: []string{"article_enrichment_pending"}}
	folder := "UFC Fight Night 254 Vettori vs. Dolidze 2 (2025)"
	file := File{RootID: "test", RootPath: t.TempDir(), Path: folder + "/Featurettes/UFC Fight Night 254 Vettori vs. Dolidze 2 - Prelims.mkv", Size: 100}
	cache := map[string]matching.Preview{}
	identified := Identify(file, []catalogue.Event{event}, cache)
	require.Equal(t, "confirmed", identified.State, identified.Reason)
	require.Equal(t, event.ID, identified.EventID)
	require.Equal(t, "prelims", identified.Component)
	require.NotEqual(t, "matched", matching.Match(folder, []catalogue.Event{event}).Status, "acquisition policy must remain unchanged")
	for _, reason := range []string{"article_list_date_conflict", "competing_ufc.fight_night", "article_identity_conflict"} {
		conflict := event
		conflict.Review = append([]string{reason}, event.Review...)
		require.Equal(t, "review", Identify(file, []catalogue.Event{conflict}, map[string]matching.Preview{}).State)
	}
	duplicate := event
	duplicate.ID = "another"
	require.Equal(t, "review", Identify(file, []catalogue.Event{event, duplicate}, map[string]matching.Preview{}).State)
	file.Path = strings.ReplaceAll(file.Path, "(2025)", "(2024)")
	require.Equal(t, "review", Identify(file, []catalogue.Event{event}, cache).State, "cached titles must not leak a different folder year")
}

func TestPendingNumberedLibraryUsesReportedUFC315Names(t *testing.T) {
	date := "2025-05-10"
	event := catalogue.Event{ID: "ufc-315", Values: catalogue.Values{Name: "UFC 315: Muhammad vs. Della Maddalena", Type: "numbered", Date: &date, Status: "unknown"}, Review: []string{"article_enrichment_pending"}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "315", Current: true}}}
	cache := map[string]matching.Preview{}
	for _, component := range []struct{ suffix, key string }{{"Early Prelims", "early_prelims"}, {"Prelims", "prelims"}} {
		file := File{RootID: "test", RootPath: t.TempDir(), Path: "UFC 315 Muhammad vs. Della Maddalena (2025)/Featurettes/UFC 315 Muhammad vs. Della Maddalena - " + component.suffix + ".mkv", Size: 100}
		identified := Identify(file, []catalogue.Event{event}, cache)
		require.Equal(t, "confirmed", identified.State, identified.Reason)
		require.Equal(t, event.ID, identified.EventID)
		require.Equal(t, component.key, identified.Component)
	}
}

func TestReadOnlyScriptLayout(t *testing.T) {
	base := t.TempDir()
	rootPath := filepath.Join(base, "archive")
	eventFolder := "UFC 173 Barao vs. Dillashaw (2014)"
	nightFolder := "UFC Fight Night 254 Vettori vs. Dolidze 2 (2025)"
	paths := []string{filepath.Join(eventFolder, eventFolder+".mkv"), filepath.Join(eventFolder, "Featurettes", eventFolder+" - Prelims.mkv"), filepath.Join(eventFolder, "Featurettes", eventFolder+" - Early Prelims.mp4"), filepath.Join(nightFolder, nightFolder+".mkv")}
	for _, relative := range paths {
		full := filepath.Join(rootPath, relative)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0700))
		require.NoError(t, os.WriteFile(full, []byte("owned temporary media fixture"), 0600))
	}
	outside := filepath.Join(base, "outside")
	require.NoError(t, os.MkdirAll(outside, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "not-scanned.mkv"), []byte("outside"), 0600))
	if err := os.Symlink(outside, filepath.Join(rootPath, "linked-outside")); err != nil {
		t.Log("Native symlink creation unavailable; symlink fixture case not run")
	}
	date, nightDate := "2014-05-24", "2025-03-15"
	events := []catalogue.Event{{ID: "numbered", Values: catalogue.Values{Name: "UFC 173", Date: &date, Type: "numbered", Status: "completed"}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "173", Current: true, Verified: true}}}, {ID: "night", Values: catalogue.Values{Name: "UFC Fight Night: Vettori vs. Dolidze 2", Date: &nightDate, Type: "fight_night", Status: "unknown"}}}
	root := Root{ID: "root", Path: rootPath}
	files, complete := walkRoot(context.Background(), root, 100)
	require.True(t, complete)
	require.Len(t, files, 4)
	components := map[string]int{}
	cache := map[string]matching.Preview{}
	for _, file := range files {
		identified := Identify(file, events, cache)
		require.Equal(t, "confirmed", identified.State, identified.Reason)
		require.Equal(t, "unknown", identified.Resolution)
		components[identified.Component]++
		data, err := os.ReadFile(filepath.Join(rootPath, filepath.FromSlash(file.Path)))
		require.NoError(t, err)
		require.Equal(t, "owned temporary media fixture", string(data))
	}
	require.Equal(t, map[string]int{"main_card": 2, "prelims": 1, "early_prelims": 1}, components)
	file := File{RootID: "root", RootPath: rootPath, Path: eventFolder + "/UFC 999 (2014).mkv", Size: 10}
	require.Equal(t, "review", Identify(file, events, cache).State)
	file.Path = eventFolder + "/Featurettes/Mystery.mkv"
	require.Equal(t, "extra", Identify(file, events, cache).State)
	require.NoError(t, os.Rename(rootPath, filepath.Join(base, "offline")))
	_, complete = walkRoot(context.Background(), root, 100)
	require.False(t, complete)
}

func TestNumberedRematchLibraryIdentity(t *testing.T) {
	date, priorDate := "2024-06-29", "2023-11-11"
	events := []catalogue.Event{
		{ID: "303", Values: catalogue.Values{Name: "UFC 303: Pereira vs. Procházka 2", Type: "numbered", Date: &date, Status: "unknown", Headliners: []string{"Pereira", "Procházka"}}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "303", Verified: true, Current: true}}},
		{ID: "295", Values: catalogue.Values{Name: "UFC 295: Procházka vs. Pereira", Type: "numbered", Date: &priorDate, Status: "unknown", Headliners: []string{"Procházka", "Pereira"}}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "295", Verified: true, Current: true}}},
	}
	folder := "UFC 303 Pereira vs. Prochazka 2 (2024)"
	before := matching.Match(folder, events)
	require.Equal(t, "ambiguous", before.Status)
	for _, item := range []struct{ suffix, component string }{{"/" + folder + ".mkv", "main_card"}, {"/Featurettes/" + folder + " - Prelims.mkv", "prelims"}, {"/Featurettes/" + folder + " - Early Prelims.mkv", "early_prelims"}} {
		file := Identify(File{RootID: "fixture", RootPath: t.TempDir(), Path: folder + item.suffix, Size: 10}, events, map[string]matching.Preview{})
		require.Equal(t, "confirmed", file.State, file.Reason)
		require.Equal(t, "303", file.EventID)
		require.Equal(t, "event:303", file.Group)
		require.Equal(t, item.component, file.Component)
	}
	conflict := events[0]
	conflict.ID = "duplicate"
	require.Equal(t, "ambiguous", matching.MatchLibrary(folder, append(events, conflict)).Status)
	require.NotEqual(t, "matched", matching.MatchLibrary("UFC 303 UFC 295", events).Status)
}
