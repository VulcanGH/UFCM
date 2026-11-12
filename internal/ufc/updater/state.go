// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package updater

import (
	"context"

	"github.com/autobrr/autobrr/internal/domain"
)

// State is the durable singleton update preference and release journal.
type State struct {
	Revision    int64                       `json:"revision"`
	AutoInstall bool                        `json:"auto_install"`
	Highwater   int64                       `json:"-"`
	ID          string                      `json:"id"`
	Version     string                      `json:"version"`
	Phase       string                      `json:"phase"`
	Message     string                      `json:"message"`
	Envelope    Envelope                    `json:"-"`
	FailedID    string                      `json:"-"`
	Notice      *domain.NotificationPayload `json:"-"`
}

// Status adds installation eligibility without exposing paths or trust material.
type Status struct {
	State
	Current    string `json:"current"`
	Supported  bool   `json:"supported"`
	Reason     string `json:"reason"`
	ReleaseURL string `json:"release_url"`
}

type repository interface {
	UpdateState(context.Context) (State, error)
	SaveUpdateState(context.Context, State) (State, error)
}
