// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"context"
	"time"

	"github.com/autobrr/autobrr/pkg/errors"
)

// LibraryIndex is a list snapshot used locally to select only observed historical events.
type LibraryIndex struct {
	Revision int64
	Facts    []Facts
}

// LibraryList reads the existing list source without resolving/enriching all articles.
func (w *Wikipedia) LibraryList(ctx context.Context) (*LibraryIndex, error) {
	parsed, err := w.parse(ctx, w.listRevision, "List of UFC events")
	if err != nil {
		return nil, err
	}
	rows, err := parseList(parsed.Parse.Text, parsed.Parse.RevID)
	if err != nil {
		return nil, err
	}
	index := &LibraryIndex{Revision: parsed.Parse.RevID, Facts: []Facts{}}
	for _, row := range rows {
		if !row.Blocked {
			f := row.Facts
			addName(&f, f.Title, true)
			index.Facts = append(index.Facts, f)
		}
	}
	return index, nil
}

// ResolveLibrary resolves only selected published list titles, without article enrichment.
func (w *Wikipedia) ResolveLibrary(ctx context.Context, index *LibraryIndex, titles []string) (*Snapshot, error) {
	if index == nil || len(titles) > 2500 {
		return nil, errors.New("invalid library metadata selection")
	}
	wanted := map[string]bool{}
	for _, title := range titles {
		wanted[title] = true
	}
	selected := []Facts{}
	published := []string{}
	for _, f := range index.Facts {
		if wanted[f.Title] {
			selected = append(selected, f)
			published = append(published, f.Title)
			delete(wanted, f.Title)
		}
	}
	if len(wanted) != 0 {
		return nil, errors.New("library selection is not in the source list")
	}
	pages, problems, err := w.resolve(ctx, published)
	if err != nil {
		return nil, err
	}
	snapshot := &Snapshot{ListRevision: index.Revision, Discovered: len(selected), Facts: []Facts{}, Diagnostics: []Diagnostic{}}
	seen := map[int64]bool{}
	for _, f := range selected {
		page, ok := pages[f.Title]
		if !ok || problems[f.Title] != "" {
			addDiagnostic(snapshot, Diagnostic{Name: f.Name, Title: f.Title, Reason: "library_source_identity_unresolved"})
			continue
		}
		if seen[page.PageID] {
			continue
		}
		seen[page.PageID] = true
		f.LinkedTitle = f.Title
		f.Title = page.Title
		f.PageID = page.PageID
		f.articleTarget = page.Revisions[0].RevID
		if pinned := w.articleRevisions[page.PageID]; pinned > 0 {
			f.articleTarget = pinned
		}
		f.Reference = pageKey(page.PageID)
		f.RetrievedAt = time.Now().UTC()
		addName(&f, page.Title, true)
		if err := ValidateFacts(f); err != nil {
			continue
		}
		snapshot.Facts = append(snapshot.Facts, f)
	}
	return snapshot, nil
}
