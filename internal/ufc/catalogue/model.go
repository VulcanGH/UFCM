// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package catalogue owns the local acquisition catalogue and its source reader.
package catalogue

import (
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
)

// Alias keeps independent identifier namespaces and previous billing distinct.
type Alias struct {
	Namespace string `json:"namespace"`
	Value     string `json:"value"`
	Current   bool   `json:"current"`
	Verified  bool   `json:"verified"`
	Manual    bool   `json:"manual,omitempty"`
}

// Facts contains source suggestions, without a local canonical identity.
type Facts struct {
	articleTarget   int64
	articleFetched  bool
	forceArticle    bool
	Reference       string    `json:"reference"`
	LinkedTitle     string    `json:"linked_title,omitempty"`
	PageID          int64     `json:"page_id"`
	ArticleRevision int64     `json:"article_revision"`
	ListRevision    int64     `json:"list_revision"`
	RetrievedAt     time.Time `json:"retrieved_at"`
	Title           string    `json:"title"`
	Name            string    `json:"name"`
	Date            *string   `json:"date"`
	Type            string    `json:"type"`
	Headliners      []string  `json:"headliners,omitempty"`
	Status          string    `json:"status"`
	Aliases         []Alias   `json:"aliases"`
	Review          []string  `json:"review,omitempty"`
	OverallSequence string    `json:"overall_sequence,omitempty"`
	Section         string    `json:"section"`
}

// Diagnostic retains an unresolved row without inventing an event identity.
type Diagnostic struct {
	Name   string `json:"name"`
	Title  string `json:"title,omitempty"`
	Reason string `json:"reason"`
}

// Snapshot separates complete discovery from optional article enrichment.
type Snapshot struct {
	AliasTotal         int          `json:"alias_total,omitempty"`
	AliasProcessed     int          `json:"alias_processed,omitempty"`
	ListRows           []Facts      `json:"list_rows,omitempty"`
	DiagnosticsOmitted int          `json:"diagnostics_omitted"`
	ListRevision       int64        `json:"list_revision"`
	Facts              []Facts      `json:"facts"`
	Diagnostics        []Diagnostic `json:"diagnostics"`
	Discovered         int          `json:"discovered"`
}

func addDiagnostic(snapshot *Snapshot, d Diagnostic) {
	if len(snapshot.Diagnostics) < 100 {
		snapshot.Diagnostics = append(snapshot.Diagnostics, d)
	} else {
		snapshot.DiagnosticsOmitted++
	}
}

// Normalize retains Unicode display text separately from lookup values.
func Normalize(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(norm.NFKC.String(value))), " ")
}

func addReview(f *Facts, reason string) {
	for _, v := range f.Review {
		if v == reason {
			return
		}
	}
	if len(f.Review) < 16 {
		f.Review = append(f.Review, reason)
	}
}

func addAlias(f *Facts, a Alias) {
	for _, v := range f.Aliases {
		if v.Namespace == a.Namespace && Normalize(v.Value) == Normalize(a.Value) && v.Current == a.Current {
			return
		}
	}
	if len(f.Aliases) < 32 {
		f.Aliases = append(f.Aliases, a)
	}
}
