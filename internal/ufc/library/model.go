// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package library catalogues existing media without changing it.
package library

// Root is a configured or retained location and its last scan health.
type Root struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Enabled bool   `json:"enabled"`
	State   string `json:"state"`
	Message string `json:"message"`
}

// Settings stores explicit root choices and durable scan status.
type Settings struct {
	Revision int64   `json:"revision"`
	Roots    []Root  `json:"roots"`
	Running  bool    `json:"running"`
	RunID    string  `json:"run_id"`
	LastScan *string `json:"last_scan"`
	Message  string  `json:"message"`
}

// SettingsEdit replaces configured paths at an expected revision.
type SettingsEdit struct {
	Revision int64    `json:"revision"`
	Paths    []string `json:"paths"`
}

// File is a metadata observation plus protected effective identification.
type File struct {
	ID                 string `json:"id"`
	Revision           int64  `json:"revision"`
	RootID             string `json:"root_id"`
	RootPath           string `json:"root_path"`
	Path               string `json:"path"`
	Size               int64  `json:"size"`
	Modified           string `json:"modified"`
	EventID            string `json:"event_id"`
	Component          string `json:"component"`
	Resolution         string `json:"resolution"`
	State              string `json:"state"`
	Reason             string `json:"reason"`
	Manual             bool   `json:"manual"`
	Present            bool   `json:"present"`
	Available          bool   `json:"available"`
	Group              string `json:"group"`
	DetectedEvent      string `json:"detected_event"`
	DetectedComponent  string `json:"detected_component"`
	DetectedResolution string `json:"detected_resolution"`
	DetectedState      string `json:"detected_state"`
	DetectedReason     string `json:"detected_reason"`
	Actor              string `json:"-"`
}

// FileEdit confirms or ignores an observation, or restores automatic identification.
type FileEdit struct {
	Revision   int64  `json:"revision"`
	Mode       string `json:"mode"`
	EventID    string `json:"event_id"`
	Component  string `json:"component"`
	Resolution string `json:"resolution"`
}

// Group is a condensed event/folder summary, not one row per file.
type Group struct {
	Wanted      []string          `json:"wanted"`
	ID          string            `json:"id"`
	EventID     string            `json:"event_id"`
	Name        string            `json:"name"`
	Date        *string           `json:"date"`
	Files       int               `json:"files"`
	Review      int               `json:"review"`
	Unavailable int               `json:"unavailable"`
	Extras      int               `json:"extras"`
	Locations   int               `json:"locations"`
	Components  map[string]string `json:"components"`
}

// View exposes scan status, compact groups and effective local coverage.
type View struct {
	Wanted     map[string][]string `json:"wanted"`
	Settings   *Settings           `json:"settings"`
	Groups     []Group             `json:"groups"`
	Configured bool                `json:"configured"`
}

// FilePage bounds expanded file rows.
type FilePage struct {
	Files []File `json:"files"`
	Total int    `json:"total"`
}

// EventOption supports manual binding to any retained catalogue event.
type EventOption struct {
	ID   string  `json:"id"`
	Name string  `json:"name"`
	Date *string `json:"date"`
}

// Accepted acknowledges a durable scan without blocking the browser.
type Accepted struct {
	RunID string `json:"run_id"`
}
