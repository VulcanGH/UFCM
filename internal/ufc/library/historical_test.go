// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package library

import (
	"fmt"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/matching"

	"github.com/stretchr/testify/require"
)

func TestHistoricalBillingAliasesAndNumberedYears(t *testing.T) {
	for _, item := range []struct{ number, title, date string }{
		{"1", "The Beginning", "1993-11-12"}, {"10", "The Tournament", "1996-07-12"},
		{"100", "Lesnar vs. Mir 2", "2009-07-11"}, {"101", "Declaration", "2009-08-08"},
		{"109", "Relentless", "2010-02-06"}, {"11", "The Proving Ground", "1996-09-20"}, {"112", "Invincible", "2010-04-10"},
	} {
		t.Run(item.number, func(t *testing.T) {
			date := item.date
			event := catalogue.Event{ID: item.number, Values: catalogue.Values{Name: "UFC " + item.number, Type: "numbered", Date: &date, Status: "unknown"}, Review: []string{"article_enrichment_pending"}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: item.number, Current: true}, {Namespace: "title", Value: "UFC " + item.number + ": " + item.title, Current: true, Verified: true}}}
			folder := fmt.Sprintf("UFC %s %s (%s)", item.number, item.title, date[:4])
			file := File{RootID: "fixture", RootPath: "archive", Path: folder + "/" + folder + ".mp4", Size: 100}
			for _, aliases := range [][]catalogue.Alias{event.Aliases, event.Aliases[:1], event.Aliases[1:]} {
				event.Aliases = aliases
				got := Identify(file, []catalogue.Event{event}, map[string]matching.Preview{})
				require.Equal(t, "confirmed", got.State, got.Reason)
				require.Equal(t, event.ID, got.EventID)
				require.Equal(t, "main_card", got.Component)
				require.NotEqual(t, "matched", matching.Match(folder, []catalogue.Event{event}).Status)
			}
		})
	}
}

func TestNumberedLibraryYearAndAliasConflictsStayInReview(t *testing.T) {
	date := "1993-11-12"
	event := catalogue.Event{ID: "one", Values: catalogue.Values{Name: "UFC 1", Type: "numbered", Date: &date}, Review: []string{"article_enrichment_pending"}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "1", Current: true}}}
	file := File{RootID: "fixture", RootPath: "archive", Path: "UFC 1 The Beginning (1993)/UFC 1 The Beginning (1993).mp4", Size: 100}
	other := event
	other.ID = "conflicting-owner"
	got := Identify(file, []catalogue.Event{event, other}, map[string]matching.Preview{})
	require.Equal(t, "review", got.State)
	for _, year := range []string{"1994", "1996"} {
		file.Path = fmt.Sprintf("UFC 1 The Beginning (%s).mp4", year)
		require.Equal(t, "review", Identify(file, []catalogue.Event{event}, map[string]matching.Preview{}).State)
	}
	event.Aliases[0].Current = false
	file.Path = "UFC 1 The Beginning (1993).mp4"
	require.Equal(t, "review", Identify(file, []catalogue.Event{event}, map[string]matching.Preview{}).State)
}

type unreadableDirectory struct{ fs.FS }

func (f unreadableDirectory) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == "b-unreadable" {
		return nil, os.ErrPermission
	}
	return fs.ReadDir(f.FS, name)
}

func TestUnreadableFolderAndUnknownTitleDoNotStopLaterMatches(t *testing.T) {
	filesystem := unreadableDirectory{fstest.MapFS{
		"a-unknown/unknown - Prelims.1080p.mkv":  {Data: []byte("fixture")},
		"b-unreadable/private.mkv":               {Data: []byte("fixture")},
		"c-later/UFC 1 The Beginning (1993).mp4": {Data: []byte("fixture")},
	}}
	files, complete := walkFiles(t.Context(), Root{ID: "fixture", Path: "archive"}, filesystem, 100)
	require.False(t, complete, "an unreadable location must prevent missing-file inference")
	require.Len(t, files, 2)
	date := "1993-11-12"
	event := catalogue.Event{ID: "one", Values: catalogue.Values{Name: "UFC 1", Type: "numbered", Date: &date}, Review: []string{"article_enrichment_pending"}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "1", Current: true}}}
	cache := map[string]matching.Preview{}
	unknown := Identify(files[0], []catalogue.Event{event}, cache)
	require.Equal(t, "review", unknown.State)
	require.Equal(t, "prelims", unknown.Component)
	require.Equal(t, "1080p", unknown.Resolution)
	require.Equal(t, "confirmed", Identify(files[1], []catalogue.Event{event}, cache).State)
}
