// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package acquisition tracks explicitly authorised manual downloads durably.
package acquisition

import (
	"github.com/autobrr/autobrr/internal/ufc/matching"
	"github.com/autobrr/autobrr/pkg/errors"
)

// ErrAutomaticDeferred retains feed work while infrastructure or Library evidence is unavailable.
var ErrAutomaticDeferred = errors.New("automatic acquisition is waiting")

// Job is durable submission intent and remote download state, without credentials.
type Job struct {
	Decision         *Decision `json:"decision,omitempty"`
	EventRevision    int64     `json:"-"`
	SettingsRevision int64     `json:"-"`
	Revision         int64     `json:"revision"`
	ID               string    `json:"id"`
	EventID          string    `json:"event_id"`
	Component        string    `json:"component"`
	Title            string    `json:"title"`
	Source           string    `json:"source"`
	ClientID         int32     `json:"client_id"`
	ClientName       string    `json:"client_name"`
	Category         string    `json:"category"`
	State            string    `json:"state"`
	RemoteID         string    `json:"remote_id"`
	Percent          float64   `json:"percent"`
	Message          string    `json:"message"`
	ReleaseID        int64     `json:"release_id"`
	CreatedAt        string    `json:"created_at"`
	UpdatedAt        string    `json:"updated_at"`
	Key              string    `json:"-"`
	Endpoint         string    `json:"-"`
	Storage          string    `json:"-"`
	Actor            string    `json:"-"`
	Indexer          string    `json:"-"`
	Size             uint64    `json:"-"`
	Resolution       string    `json:"-"`
	MediaSource      string    `json:"-"`
	Codec            string    `json:"-"`
}

// Submit selects an owned candidate and existing enabled SABnzbd connection.
type Submit struct {
	Automated        bool   `json:"-"`
	ManualReason     string `json:"manual_reason,omitempty"`
	EventRevision    int64  `json:"event_revision,omitempty"`
	SettingsRevision int64  `json:"settings_revision,omitempty"`
	CandidateID      string `json:"candidate_id"`
	ClientID         int32  `json:"client_id"`
	Category         string `json:"category"`
	Actor            string `json:"-"`
}

// ClientOption exposes only a configured client's identity.
type ClientOption struct {
	ID   int32  `json:"id"`
	Name string `json:"name"`
}

// Decision retains automatic evidence separately from an explicit manual assignment.
type Decision struct {
	Automated        bool             `json:"automated,omitempty"`
	Manual           bool             `json:"manual"`
	Reason           string           `json:"reason,omitempty"`
	Actor            string           `json:"actor"`
	At               string           `json:"at"`
	EventID          string           `json:"event_id"`
	Component        string           `json:"component"`
	EventRevision    int64            `json:"event_revision"`
	SettingsRevision int64            `json:"settings_revision"`
	Automatic        matching.Preview `json:"automatic"`
}

// Recovery binds an existing remote job without resubmitting or cancelling anything.
type Recovery struct {
	Revision int64  `json:"revision"`
	RemoteID string `json:"remote_id"`
	Confirm  bool   `json:"confirm"`
	Actor    string `json:"-"`
}
