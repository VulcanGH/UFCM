// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

// Package automation connects scheduled feeds to the existing UFC acquisition workflow.
package automation

import (
	"time"

	"github.com/autobrr/autobrr/internal/feed"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
)

// Settings contains the existing application's revisioned acquisition switch.
type Settings struct {
	Revision int  `json:"revision"`
	Enabled  bool `json:"enabled"`
}

// Item is an internal durable feed candidate; its link never enters a browser response.
type Item struct {
	ID, ConfigKey, EventID, Component string
	FeedID                            int
	Release                           feed.SearchItem
	Attempts                          int
}

// Notice is independent of download and copy state.
type Notice struct {
	ID, Event, Title, Message string
	Attempts                  int
}

// Deadline allows 48 hours after the end of a date-only event day.
func Deadline(event catalogue.Event) (time.Time, bool) {
	if event.Values.Date == nil {
		return time.Time{}, false
	}
	day, err := time.Parse("2006-01-02", *event.Values.Date)
	return day.Add(72 * time.Hour), err == nil
}

// CanAdmit excludes historical, cancelled, undated and protected-deleted events.
func CanAdmit(event catalogue.Event, now time.Time) bool {
	return !event.Corrections.Deleted && event.Values.Status == "scheduled" && event.Values.Date != nil && *event.Values.Date >= now.UTC().Format("2006-01-02")
}

// Active requires trustworthy dated event facts and an unexpired acquisition window.
func Active(event catalogue.Event, now time.Time) bool {
	deadline, ok := Deadline(event)
	return ok && now.Before(deadline) && !event.Corrections.Deleted && (event.Values.Status == "scheduled" || event.Values.Status == "completed")
}

// ClosedWindow separates retirement from temporary postponement or missing metadata.
func ClosedWindow(event catalogue.Event, now time.Time) bool {
	deadline, known := Deadline(event)
	return event.Corrections.Deleted || event.Values.Status == "cancelled" || (known && !now.Before(deadline))
}
