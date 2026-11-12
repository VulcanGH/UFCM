// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package matching

import (
	"strings"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
)

func sourceNumber(event catalogue.Event) string {
	if event.Source.PageID <= 0 || event.Values.Type != "numbered" || event.Source.Type != "numbered" {
		return ""
	}
	title := folded(strings.TrimSpace(event.Source.Title))
	indices := numbered.FindStringSubmatchIndex(title)
	if len(indices) == 0 || indices[0] != 0 {
		return ""
	}
	return title[indices[2]:indices[3]]
}

func libraryCatalogue(events []catalogue.Event) []catalogue.Event {
	result := append([]catalogue.Event{}, events...)
	removed := map[string]bool{}
	owners := map[Identifier]map[string]bool{}
	for i, event := range result {
		number := sourceNumber(event)
		family := words(event.Source.Title)
		knownFamily := number != "" || event.Source.PageID > 0 && event.Source.Type == "fight_night" && event.Values.Type == "fight_night" && (strings.HasPrefix(family, "ufc fight night") || strings.HasPrefix(family, "ufc on ") || strings.HasPrefix(family, "ufc vegas ") || strings.HasPrefix(family, "ufc freedom"))
		result[i].Aliases = make([]catalogue.Alias, 0, len(event.Aliases))
		for _, alias := range event.Aliases {
			if knownFamily && !alias.Manual && alias.Namespace == "ufc.numbered" && alias.Value != number {
				removed[event.ID] = true
				continue
			}
			result[i].Aliases = append(result[i].Aliases, alias)
			if !event.Corrections.Deleted && alias.Current && strings.HasPrefix(alias.Namespace, "ufc.") {
				id := Identifier{alias.Namespace, alias.Value}
				if owners[id] == nil {
					owners[id] = map[string]bool{}
				}
				owners[id][event.ID] = true
			}
		}
	}
	for i, event := range result {
		duplicate := false
		for _, alias := range event.Aliases {
			if alias.Current && len(owners[Identifier{alias.Namespace, alias.Value}]) > 1 {
				duplicate = true
			}
		}
		result[i].Review = make([]string, 0, len(event.Review))
		for _, reason := range event.Review {
			if reason == "identifier_owned_by_existing_event" && event.Source.PageID > 0 && !duplicate {
				continue
			}
			if removed[event.ID] && (reason == "competing_ufc.numbered" || reason == "article_list_type_conflict") {
				continue
			}
			result[i].Review = append(result[i].Review, reason)
		}
	}
	return result
}

func numberedLibraryIdentity(p Preview, events []catalogue.Event) (Preview, bool) {
	var number string
	for _, id := range p.Identifiers {
		if id.Namespace == "ufc.numbered" {
			number = id.Value
		}
	}
	if number == "" {
		return p, false
	}
	if len(p.Identifiers) != 1 {
		p.Status = "needs review"
		p.Notes = append(p.Notes, "Several event identifiers appear in the title")
		return p, true
	}
	owners := []catalogue.Event{}
	trusted := false
	for _, event := range events {
		if event.Corrections.Deleted {
			continue
		}
		canonical := sourceNumber(event) == number && (event.Corrections.Aliases == nil || !event.Corrections.Aliases.Protected)
		owner := canonical
		for _, alias := range event.Aliases {
			if alias.Namespace == "ufc.numbered" && alias.Value == number && alias.Current {
				owner = true
				trusted = trusted || alias.Verified
				break
			}
		}
		if owner {
			owners = append(owners, event)
			trusted = trusted || canonical
		}
	}
	if len(owners) == 0 {
		for _, event := range events {
			for _, alias := range event.Aliases {
				if !event.Corrections.Deleted && alias.Namespace == "title" && alias.Current && alias.Verified && libraryTitleAppears(words(p.Title), alias.Value) {
					return p, false
				}
			}
		}
		p.Status, p.Candidates = "unmatched", []Candidate{}
		p.Notes = append(p.Notes, "No saved numbered event has the identity UFC "+number)
		return p, true
	}
	if !trusted {
		return p, false
	}
	p.Candidates = []Candidate{}
	for _, event := range owners {
		c := Candidate{ID: event.ID, Name: event.Values.Name, Date: event.Values.Date, Reasons: []string{"Unique current numbered-event identity: UFC " + number}, Conflicts: []string{}}
		canonical := sourceNumber(event) == number
		for _, reason := range event.Review {
			if reason == "article_enrichment_pending" || reason == "article_enrichment_unavailable" {
				continue
			}
			if canonical && (reason == "article_list_date_conflict" || reason == "article_list_headliner_conflict" || reason == "source_date_unknown_previous_retained" || reason == "source_headliners_unknown_previous_retained") {
				continue
			}
			c.Conflicts = append(c.Conflicts, "Catalogue identity warning: "+reason)
		}
		if event.Values.Type != "numbered" || event.Values.Status == "cancelled" || event.Values.Status == "postponed" {
			c.Conflicts = append(c.Conflicts, "Event type or status is incompatible")
		}
		if year := LibraryYear(p.Title); year != "" && event.Values.Date != nil && len(*event.Values.Date) >= 4 && year != (*event.Values.Date)[:4] {
			c.Conflicts = append(c.Conflicts, "Folder year differs from catalogue year")
		}
		p.Candidates = append(p.Candidates, c)
	}
	p.Status = "matched"
	if len(p.Candidates) > 1 {
		p.Status = "ambiguous"
	} else if len(p.Candidates[0].Conflicts) > 0 {
		p.Status = "needs review"
	}
	return p, true
}
