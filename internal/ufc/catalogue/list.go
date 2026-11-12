// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"sort"
	"strconv"
	"strings"
)

// RecentCompletedLimit bounds the routinely refreshed and displayed completed window.
const RecentCompletedLimit = 10

// EventSummary is the read-only acquisition catalogue presentation.
type EventSummary struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Date        *string  `json:"date"`
	Aliases     []Alias  `json:"aliases"`
	NeedsReview bool     `json:"needs_review"`
	Review      []string `json:"review"`
	SourceURL   string   `json:"source_url"`
}

// EventList groups upcoming and the ten latest explicitly completed events.
type EventList struct {
	Upcoming        []EventSummary `json:"upcoming"`
	RecentCompleted []EventSummary `json:"recent_completed"`
}

// EventPage is a bounded view over the permanent catalogue, independent of selection.
type EventPage struct {
	Events []EventSummary `json:"events"`
	Total  int            `json:"total"`
}

// Browse filters names, aliases, dates and published sequence before paging results.
func Browse(events []Event, search string, offset int) *EventPage {
	return BrowseWindow(events, search, offset, 50, false, "")
}

// BrowseWindow selects a bounded catalogue page, optionally restricted to published history.
func BrowseWindow(events []Event, search string, offset, limit int, pastOnly bool, today string) *EventPage {
	selected := []Event{}
	needle := Normalize(search)
	for _, event := range events {
		if event.Corrections.Deleted {
			continue
		}
		if pastOnly && ((event.WorkspacePast != nil && !*event.WorkspacePast) || (event.WorkspacePast == nil && event.Values.Status != "completed" && !(event.Source.Section == "past events" && (event.Values.Date == nil || *event.Values.Date <= today)))) {
			continue
		}
		text := event.Values.Name + " " + strings.Join(event.Values.Headliners, " ") + " " + event.Source.OverallSequence
		if event.Values.Date != nil {
			text += " " + *event.Values.Date
		}
		for _, alias := range event.Aliases {
			text += " " + alias.Value
		}
		if needle == "" || strings.Contains(Normalize(text), needle) {
			selected = append(selected, event)
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		a, b := selected[i], selected[j]
		if a.Values.Date != nil && b.Values.Date != nil && *a.Values.Date != *b.Values.Date {
			return *a.Values.Date > *b.Values.Date
		}
		if (a.Values.Date != nil) != (b.Values.Date != nil) {
			return a.Values.Date != nil
		}
		if a.Values.Name != b.Values.Name {
			return a.Values.Name < b.Values.Name
		}
		return a.ID < b.ID
	})
	page := &EventPage{Events: []EventSummary{}, Total: len(selected)}
	offset = max(0, min(offset, len(selected)))
	limit = max(1, min(limit, 2500))
	for _, event := range selected[offset:min(offset+limit, len(selected))] {
		page.Events = append(page.Events, Summarize(event))
	}
	return page
}

// Summarize exposes effective values while retaining alias and source provenance.
func Summarize(event Event) EventSummary {
	sourceURL := "https://en.wikipedia.org/?curid=" + strconv.FormatInt(event.Source.PageID, 10)
	if event.Source.PageID == 0 {
		sourceURL = ""
	}
	if event.Source.ArticleRevision > 0 {
		sourceURL = "https://en.wikipedia.org/w/index.php?oldid=" + strconv.FormatInt(event.Source.ArticleRevision, 10)
	}
	return EventSummary{ID: event.ID, Name: event.Values.Name, Date: event.Values.Date, Aliases: event.Aliases, NeedsReview: len(event.Review) > 0, Review: event.Review, SourceURL: sourceURL}
}
