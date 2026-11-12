// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import "github.com/autobrr/autobrr/pkg/errors"

// QualityPreferences resolves accepted resolutions independently of component intent.
type QualityPreferences struct {
	SettingsRevision int64    `json:"settings_revision"`
	Defaults         []string `json:"defaults"`
	Override         []string `json:"override"`
	Effective        []string `json:"effective"`
}

// QualityEdit selects inherited or explicitly overridden resolutions.
type QualityEdit struct {
	Mode        string   `json:"mode"`
	Resolutions []string `json:"resolutions,omitempty"`
}

// ValidateResolutions accepts a nonempty selection of supported resolution labels.
func ValidateResolutions(values []string) error {
	if len(values) < 1 || len(values) > 5 {
		return errors.Wrap(ErrInvalidEdit, "choose at least one accepted resolution")
	}
	seen := map[string]bool{}
	for _, value := range values {
		if (value != "2160p" && value != "1080p" && value != "1080i" && value != "720p" && value != "480p") || seen[value] {
			return errors.Wrap(ErrInvalidEdit, "invalid or duplicate resolution")
		}
		seen[value] = true
	}
	return nil
}
