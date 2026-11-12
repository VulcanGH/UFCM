// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package matching

import (
	"strconv"
	"strings"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
)

// LibraryYear reads an explicit parenthesised year, never the current year.
func LibraryYear(title string) string {
	for start := strings.Index(title, "("); start >= 0; start = strings.Index(title, "(") {
		title = title[start+1:]
		end := strings.Index(title, ")")
		if end < 0 {
			return ""
		}
		value := title[:end]
		year, err := strconv.Atoi(value)
		if err == nil && len(value) == 4 && year >= 1900 && year <= 2099 {
			return value
		}
		title = title[end+1:]
	}
	return ""
}

func headlinerTitle(name string) string {
	parts := strings.Fields(words(name))
	if len(parts) > 0 && parts[0] == "ufc" {
		parts = parts[1:]
	}
	for _, prefix := range [][]string{{"fight", "night"}, {"on", "espn"}, {"on", "abc"}, {"vegas"}, {"freedom"}} {
		if len(parts) >= len(prefix) && strings.Join(parts[:len(prefix)], " ") == strings.Join(prefix, " ") {
			parts = parts[len(prefix):]
			break
		}
	}
	if len(parts) > 0 {
		if _, err := strconv.Atoi(parts[0]); err == nil {
			parts = parts[1:]
		}
	}
	return strings.Join(parts, " ")
}

func libraryIdentity(title string, events []catalogue.Event) Preview {
	events = libraryCatalogue(events)
	p := match(title, events, true, nil)
	if numbered, handled := numberedLibraryIdentity(p, events); handled {
		return numbered
	}
	if p.Status == "matched" {
		return p
	}
	// Rematches share surnames; weaker candidates must not obscure a verified identity.
	confident := []Candidate{}
	for _, candidate := range p.Candidates {
		if len(candidate.Conflicts) == 0 {
			confident = append(confident, candidate)
		}
	}
	if len(confident) == 1 {
		p.Candidates = confident
		p.Status = "matched"
		return p
	}
	year := LibraryYear(title)
	if year == "" {
		return p
	}
	possible := []Candidate{}
	normalized := words(title)
	for _, e := range events {
		if e.Corrections.Deleted || libraryIdentityConflict(e.Review) || e.Values.Date == nil || len(*e.Values.Date) < 4 || (*e.Values.Date)[:4] != year || e.Values.Status == "cancelled" || e.Values.Status == "postponed" {
			continue
		}
		reasons := []string{}
		for _, alias := range e.Aliases {
			if !alias.Current {
				continue
			}
			if alias.Namespace == "ufc.numbered" && e.Values.Type == "numbered" && (alias.Verified || len(e.Review) > 0) {
				for _, id := range p.Identifiers {
					if id.Namespace == alias.Namespace && id.Value == alias.Value {
						reasons = append(reasons, "Current numbered-event alias and explicit year agree")
					}
				}
			}
			if alias.Namespace == "title" && (alias.Verified || len(e.Review) > 0) && libraryTitleAppears(normalized, alias.Value) {
				reasons = append(reasons, "Current title alias and explicit year agree: "+alias.Value)
			}
		}
		if libraryTitleAppears(normalized, e.Values.Name) {
			reasons = append(reasons, "Effective event title and explicit year agree")
		}
		core := headlinerTitle(e.Values.Name)
		if contains(core, "vs") && contains(normalized, core) {
			reasons = append(reasons, "Headliner title and explicit year agree")
		}
		if len(reasons) == 0 {
			continue
		}
		conflict := false
		for _, id := range p.Identifiers {
			for _, other := range events {
				for _, a := range other.Aliases {
					if !other.Corrections.Deleted && a.Current && a.Namespace == id.Namespace && a.Value == id.Value && other.ID != e.ID {
						conflict = true
					}
				}
			}
		}
		if !conflict {
			possible = append(possible, Candidate{ID: e.ID, Name: e.Values.Name, Date: e.Values.Date, Reasons: reasons, Conflicts: []string{}})
		}
	}
	if len(possible) > 0 {
		p.Candidates = possible
		p.Status = "matched"
		if len(possible) > 1 {
			p.Status = "ambiguous"
		}
	}
	return p
}

func libraryTitleAppears(title, name string) bool {
	name = words(name)
	if len(strings.Fields(name)) < 3 || name == "ufc fight night" || name == "ufc on espn" || name == "ufc on abc" {
		return false
	}
	return contains(title, name)
}

func libraryIdentityConflict(reasons []string) bool {
	for _, reason := range reasons {
		if reason != "article_enrichment_pending" && reason != "article_enrichment_unavailable" {
			return true
		}
	}
	return false
}
