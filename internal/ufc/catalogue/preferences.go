// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/autobrr/autobrr/pkg/errors"
)

// ComponentPreferences separates inherited defaults from explicit event choices.
type ComponentPreferences struct {
	Defaults  []string        `json:"defaults"`
	Overrides map[string]bool `json:"overrides"`
	Wanted    []string        `json:"wanted"`
}

// Settings exposes local templates independently of metadata refresh controls.
type Settings struct {
	MainCardTerms       []string       `json:"main_card_terms"`
	Resolutions         []string       `json:"resolutions"`
	Revision            int64          `json:"revision"`
	Templates           Templates      `json:"templates"`
	EventCounts         map[string]int `json:"event_counts"`
	PeriodicSyncEnabled bool           `json:"periodic_sync_enabled"`
	SyncIntervalSeconds int            `json:"sync_interval_seconds"`
	NextDue             *string        `json:"next_due"`
}

// SettingsEdit replaces both templates at an expected settings revision.
type SettingsEdit struct {
	MainCardTerms       json.RawMessage `json:"main_card_terms,omitempty"`
	Resolutions         *[]string       `json:"resolutions,omitempty"`
	Revision            int64           `json:"revision"`
	Templates           Templates       `json:"templates"`
	PeriodicSyncEnabled *bool           `json:"periodic_sync_enabled,omitempty"`
	SyncIntervalSeconds *int            `json:"sync_interval_seconds,omitempty"`
}

// Settings reads persisted component defaults without contacting a source.
func (s *Service) Settings(ctx context.Context) (*Settings, error) {
	return s.repo.CataloguePreferences(ctx)
}

// SaveSettings changes component defaults without enabling acquisition or processing.
func (s *Service) SaveSettings(ctx context.Context, edit SettingsEdit) (*Settings, error) {
	settings, err := s.repo.SaveCataloguePreferences(ctx, edit)
	if err == nil {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	}
	return settings, err
}

// ValidateMainCardTerms bounds plain phrases without allowing regex configuration.
func ValidateMainCardTerms(terms []string) error {
	if len(terms) > 200 {
		return errors.Wrap(ErrInvalidEdit, "at most 200 exclusion terms are allowed")
	}
	seen := map[string]bool{}
	for _, term := range terms {
		normalized := strings.ToLower(strings.TrimSpace(term))
		if normalized == "" || len(term) > 100 || strings.IndexFunc(term, unicode.IsControl) >= 0 || seen[normalized] || strings.ContainsAny(term, `\[]{}()*+?^$|`) {
			return errors.Wrap(ErrInvalidEdit, "use unique plain words or phrases, up to 100 bytes each")
		}
		seen[normalized] = true
	}
	return nil
}
