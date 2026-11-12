// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"strings"
	"time"
	"unicode"

	"github.com/autobrr/autobrr/pkg/errors"
)

// Values contains the effective acquisition facts, including explicitly unknown values.
type Values struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Date       *string  `json:"date"`
	Headliners []string `json:"headliners"`
	Status     string   `json:"status"`
}

// Correction protects a field, including an explicit unknown value.
type Correction struct {
	Protected bool      `json:"protected"`
	Value     *string   `json:"value"`
	Names     []string  `json:"names,omitempty"`
	Actor     string    `json:"actor"`
	Reason    string    `json:"reason"`
	At        time.Time `json:"at"`
}

// Corrections is an allowlist of editable fields, without generic metadata properties.
type Corrections struct {
	Deleted    bool             `json:"deleted,omitempty"`
	Deletion   *Correction      `json:"deletion,omitempty"`
	Name       *Correction      `json:"name,omitempty"`
	Type       *Correction      `json:"type,omitempty"`
	Date       *Correction      `json:"date,omitempty"`
	Headliners *Correction      `json:"headliners,omitempty"`
	Status     *Correction      `json:"status,omitempty"`
	Aliases    *AliasCorrection `json:"aliases,omitempty"`
}

// Event retains an immutable local identity and latest source suggestions.
type Event struct {
	WorkspacePast *bool                 `json:"-"`
	Quality       *QualityPreferences   `json:"quality,omitempty"`
	ID            string                `json:"id"`
	Revision      int64                 `json:"revision"`
	Values        Values                `json:"values"`
	Source        Facts                 `json:"source"`
	Corrections   Corrections           `json:"corrections"`
	Aliases       []Alias               `json:"aliases"`
	Review        []string              `json:"review"`
	Components    *ComponentPreferences `json:"components,omitempty"`
}

// Templates defines locally wanted component keys by event type.
type Templates map[string][]string

// DefaultTemplates returns independent editable component defaults.
func DefaultTemplates() Templates {
	return Templates{"numbered": {"main_card", "prelims", "early_prelims"}, "fight_night": {"main_card", "prelims"}}
}

// Components resolves explicit on/off overrides without consulting metadata.
func Components(eventType string, templates Templates, overrides map[string]bool) []string {
	wanted := map[string]bool{}
	for _, key := range templates[eventType] {
		wanted[key] = true
	}
	for key, on := range overrides {
		wanted[key] = on
	}
	keys := []string{}
	for _, key := range []string{"main_card", "prelims", "early_prelims", "countdown", "embedded", "weigh_ins"} {
		if wanted[key] {
			keys = append(keys, key)
		}
	}
	return keys
}

// Merge applies source facts without resetting corrections or manual aliases.
func Merge(old Event, source Facts) (Event, error) {
	source.Reference = pageKey(source.PageID)
	if err := ValidateFacts(source); err != nil {
		return Event{}, err
	}
	if old.Source.PageID > 0 && old.Source.PageID != source.PageID {
		return Event{}, errors.New("source identity cannot change")
	}
	result := old
	result.Source = source
	result.Review = append([]string{}, source.Review...)
	v := old.Values
	if source.Name != "" {
		v.Name = source.Name
	}
	if source.Type != "" {
		v.Type = source.Type
	}
	if source.Date != nil {
		d := *source.Date
		v.Date = &d
	} else if old.Values.Date != nil {
		result.Review = append(result.Review, "source_date_unknown_previous_retained")
	}
	if len(source.Headliners) == 2 {
		v.Headliners = append([]string{}, source.Headliners...)
	} else if len(old.Values.Headliners) > 0 {
		result.Review = append(result.Review, "source_headliners_unknown_previous_retained")
	}
	if source.Status != "unknown" {
		v.Status = source.Status
	}
	if v.Status == "" {
		v.Status = "unknown"
	}
	for _, pair := range []struct {
		c    *Correction
		dest *string
	}{{old.Corrections.Name, &v.Name}, {old.Corrections.Type, &v.Type}, {old.Corrections.Status, &v.Status}} {
		if pair.c != nil && pair.c.Protected {
			if pair.c.Value == nil {
				return Event{}, errors.New("required corrected field cannot be unknown")
			}
			*pair.dest = *pair.c.Value
		}
	}
	if c := old.Corrections.Date; c != nil && c.Protected {
		v.Date = c.Value
	}
	if c := old.Corrections.Headliners; c != nil && c.Protected {
		v.Headliners = append([]string{}, c.Names...)
	}
	if err := validateValues(v); err != nil {
		return Event{}, err
	}
	result.Values = v
	result.Aliases = append([]Alias{}, source.Aliases...)
	for _, a := range old.Aliases {
		if a.Manual {
			result.Aliases = retainAlias(result.Aliases, a)
			continue
		}
		if a.Namespace == "title" && a.Value != source.Name && old.Values.Name != source.Name {
			a.Current = false
			a.Verified = false
		}
		for _, newAlias := range source.Aliases {
			if a.Namespace == newAlias.Namespace && a.Value != newAlias.Value && newAlias.Current && newAlias.Verified && strings.HasPrefix(a.Namespace, "ufc.") {
				a.Current = false
				a.Verified = false
			}
		}
		result.Aliases = retainAlias(result.Aliases, a)
	}
	if source.Status == "cancelled" || source.Status == "postponed" || v.Status == "cancelled" || v.Status == "postponed" {
		for i := range result.Aliases {
			if !result.Aliases[i].Manual {
				result.Aliases[i].Current = false
				result.Aliases[i].Verified = false
			}
		}
	}
	if len(result.Review) > 0 {
		for i := range result.Aliases {
			if !result.Aliases[i].Manual && strings.HasPrefix(result.Aliases[i].Namespace, "ufc.") {
				result.Aliases[i].Verified = false
			}
		}
	}
	deduped := []Alias{}
	for _, a := range result.Aliases {
		deduped = retainAlias(deduped, a)
	}
	result.Aliases = deduped
	if c := old.Corrections.Aliases; c != nil && c.Protected {
		result.Aliases = append([]Alias{}, c.Values...)
	}
	if len(result.Aliases) > 96 {
		return Event{}, errors.New("retained event alias limit exceeded")
	}
	return result, nil
}

func retainAlias(aliases []Alias, a Alias) []Alias {
	for i, v := range aliases {
		if v.Namespace == a.Namespace && Normalize(v.Value) == Normalize(a.Value) && v.Current == a.Current {
			if a.Manual {
				aliases[i] = a
			}
			return aliases
		}
	}
	return append(aliases, a)
}

// ValidateFacts rejects malformed source suggestions before any transaction changes.
func ValidateFacts(f Facts) error {
	if f.PageID <= 0 || f.ListRevision <= 0 || f.RetrievedAt.IsZero() || !identityTitle(f.Title) || len(f.Title) > 512 {
		return errors.New("unconfirmed or invalid Wikipedia source identity")
	}
	if len(f.Aliases) > 96 || len(f.Review) > 16 {
		return errors.New("source field limit exceeded")
	}
	for _, a := range f.Aliases {
		if a.Manual {
			return errors.New("source cannot own manual aliases")
		}
		if err := ValidateAlias(a); err != nil {
			return err
		}
	}
	return validateValues(Values{Name: f.Name, Type: f.Type, Date: f.Date, Headliners: f.Headliners, Status: f.Status})
}

// ValidateAlias bounds and validates current and historical lookup values.
func ValidateAlias(a Alias) error {
	if !validText(a.Value, 512) {
		return errors.New("invalid alias value")
	}
	switch a.Namespace {
	case "title":
	case "ufc.numbered", "ufc.fight_night", "ufc.vegas", "ufc.espn", "ufc.espn_plus", "ufc.abc", "ufc.freedom":
		for _, c := range a.Value {
			if c < '0' || c > '9' {
				if c != '.' || a.Namespace != "ufc.numbered" {
					return errors.New("invalid numbering alias")
				}
			}
		}
		if strings.Contains(a.Value, ".") && a.Value != "37.5" {
			return errors.New("unsupported fractional UFC number")
		}
	default:
		return errors.New("unsupported alias namespace")
	}
	return nil
}

func validateValues(v Values) error {
	if !validText(v.Name, 512) || (v.Type != "numbered" && v.Type != "fight_night") {
		return errors.New("invalid event name or type")
	}
	if v.Date != nil {
		if _, err := time.Parse("2006-01-02", *v.Date); err != nil {
			return errors.New("invalid event-local date")
		}
	}
	if len(v.Headliners) != 0 && len(v.Headliners) != 2 {
		return errors.New("headliners must be unknown or an ordered pair")
	}
	for _, name := range v.Headliners {
		if !validText(name, 128) {
			return errors.New("invalid headliner name")
		}
	}
	switch v.Status {
	case "unknown", "scheduled", "postponed", "cancelled", "completed":
	default:
		return errors.New("invalid event status")
	}
	return nil
}

func validText(v string, limit int) bool {
	return strings.TrimSpace(v) != "" && len(v) <= limit && !strings.ContainsFunc(v, unicode.IsControl)
}

// ValidateOverrides restricts local component preferences to supported keys.
func ValidateOverrides(overrides map[string]bool) error {
	if len(overrides) > 6 {
		return errors.New("component override limit exceeded")
	}
	for key := range overrides {
		switch key {
		case "main_card", "prelims", "early_prelims", "countdown", "embedded", "weigh_ins":
		default:
			return errors.New("unsupported component key")
		}
	}
	return nil
}

// ValidateTemplates validates the two local event-type defaults.
func ValidateTemplates(templates Templates) error {
	if len(templates) != 2 {
		return errors.New("both event component templates are required")
	}
	for _, eventType := range []string{"numbered", "fight_night"} {
		keys, ok := templates[eventType]
		if !ok {
			return errors.New("missing event component template")
		}
		seen := map[string]bool{}
		for _, key := range keys {
			if seen[key] {
				return errors.New("duplicate template component")
			}
			seen[key] = true
		}
		if err := ValidateOverrides(seen); err != nil {
			return err
		}
	}
	return nil
}
