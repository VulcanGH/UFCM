// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"context"
	"time"

	"github.com/autobrr/autobrr/pkg/errors"
)

// ErrInvalidEdit identifies a rejected manual correction.
var ErrInvalidEdit = errors.Sentinel("invalid event edit")

// ErrEventNotFound identifies an unknown local event.
var ErrEventNotFound = errors.Sentinel("event not found")

// FieldEdit distinguishes a protected value, including unknown, from source reset.
type FieldEdit struct {
	Operation string   `json:"operation"`
	Value     *string  `json:"value,omitempty"`
	Names     []string `json:"names,omitempty"`
}

// EventEdit changes only supplied fields at an expected event revision.
type EventEdit struct {
	Quality    *QualityEdit      `json:"quality,omitempty"`
	Delete     bool              `json:"delete,omitempty"`
	Revision   int64             `json:"revision"`
	Name       *FieldEdit        `json:"name,omitempty"`
	Type       *FieldEdit        `json:"type,omitempty"`
	Date       *FieldEdit        `json:"date,omitempty"`
	Headliners *FieldEdit        `json:"headliners,omitempty"`
	Status     *FieldEdit        `json:"status,omitempty"`
	Components map[string]string `json:"components,omitempty"`
	Aliases    *AliasEdit        `json:"aliases,omitempty"`
}

// ApplyEdit validates protected corrections and component choices without changing identity.
func ApplyEdit(old Event, edit EventEdit, actor string, at time.Time) (Event, error) {
	if old.Corrections.Deleted {
		return Event{}, ErrEventNotFound
	}
	if edit.Revision < 1 || (!edit.Delete && edit.Quality == nil && edit.Name == nil && edit.Type == nil && edit.Aliases == nil && edit.Date == nil && edit.Headliners == nil && edit.Status == nil && len(edit.Components) == 0) {
		return Event{}, errors.Wrap(ErrInvalidEdit, "revision and at least one field are required")
	}
	if old.Revision != edit.Revision {
		return Event{}, ErrRevisionConflict
	}
	if edit.Delete {
		if edit.Quality != nil || edit.Name != nil || edit.Type != nil || edit.Aliases != nil || edit.Date != nil || edit.Headliners != nil || edit.Status != nil || len(edit.Components) != 0 {
			return Event{}, errors.Wrap(ErrInvalidEdit, "delete must be submitted separately from corrections")
		}
		old.Corrections.Deleted = true
		old.Corrections.Deletion = &Correction{Protected: true, Actor: actor, At: at.UTC()}
		old.Aliases = []Alias{}
		return old, nil
	}
	keys := map[string]bool{}
	if q := edit.Quality; q != nil {
		if q.Mode == "custom" {
			if err := ValidateResolutions(q.Resolutions); err != nil {
				return Event{}, err
			}
		} else if q.Mode != "inherit" || len(q.Resolutions) != 0 {
			return Event{}, errors.Wrap(ErrInvalidEdit, "invalid quality preference mode")
		}
	}
	for key, state := range edit.Components {
		if state != "inherit" && state != "on" && state != "off" {
			return Event{}, errors.Wrap(ErrInvalidEdit, "invalid component preference")
		}
		keys[key] = true
	}
	if err := ValidateOverrides(keys); err != nil {
		return Event{}, errors.Wrap(ErrInvalidEdit, "%s", err.Error())
	}
	result := old
	for _, field := range []struct {
		key        string
		edit       *FieldEdit
		correction **Correction
	}{{"name", edit.Name, &result.Corrections.Name}, {"type", edit.Type, &result.Corrections.Type}, {"date", edit.Date, &result.Corrections.Date}, {"headliners", edit.Headliners, &result.Corrections.Headliners}, {"status", edit.Status, &result.Corrections.Status}} {
		if field.edit == nil {
			continue
		}
		f := field.edit
		if f.Operation != "set" && f.Operation != "reset" {
			return Event{}, errors.Wrap(ErrInvalidEdit, "unsupported field operation")
		}
		if f.Operation == "reset" && old.Source.PageID == 0 {
			return Event{}, errors.Wrap(ErrInvalidEdit, "manual event has no Wikipedia value")
		}
		if (field.key != "headliners" && f.Names != nil) || (field.key == "headliners" && f.Value != nil) || (f.Operation == "reset" && (f.Value != nil || f.Names != nil)) {
			return Event{}, errors.Wrap(ErrInvalidEdit, "field value does not match operation")
		}
		if f.Operation == "set" {
			*field.correction = &Correction{Protected: true, Value: f.Value, Names: append([]string(nil), f.Names...), Actor: actor, At: at.UTC()}
		} else {
			*field.correction = nil
		}
		switch field.key {
		case "type":
			if f.Operation == "reset" {
				result.Values.Type = old.Source.Type
			} else if f.Value != nil {
				result.Values.Type = *f.Value
			} else {
				return Event{}, errors.Wrap(ErrInvalidEdit, "event type is required")
			}
		case "name":
			if f.Operation == "reset" {
				result.Values.Name = old.Source.Name
			} else if f.Value != nil {
				result.Values.Name = *f.Value
			} else {
				return Event{}, errors.Wrap(ErrInvalidEdit, "name cannot be empty")
			}
		case "date":
			result.Values.Date = old.Source.Date
			if f.Operation == "set" {
				result.Values.Date = f.Value
			}
		case "headliners":
			result.Values.Headliners = append([]string{}, old.Source.Headliners...)
			if f.Operation == "set" {
				result.Values.Headliners = append([]string{}, f.Names...)
			}
		case "status":
			if f.Operation == "reset" {
				result.Values.Status = old.Source.Status
			} else if f.Value != nil {
				result.Values.Status = *f.Value
			} else {
				return Event{}, errors.Wrap(ErrInvalidEdit, "select a status, including unknown")
			}
		}
	}
	if a := edit.Aliases; a != nil {
		switch a.Operation {
		case "set":
			aliases, err := manualAliases(a.Values)
			if err != nil {
				return Event{}, err
			}
			result.Aliases = aliases
			result.Corrections.Aliases = &AliasCorrection{Protected: true, Values: aliases, Actor: actor, At: at.UTC()}
		case "reset":
			if old.Source.PageID == 0 || a.Values != nil {
				return Event{}, errors.Wrap(ErrInvalidEdit, "invalid alias source reset")
			}
			result.Aliases = append([]Alias{}, old.Source.Aliases...)
			result.Corrections.Aliases = nil
		default:
			return Event{}, errors.Wrap(ErrInvalidEdit, "invalid alias operation")
		}
	}
	if err := validateValues(result.Values); err != nil {
		return Event{}, errors.Wrap(ErrInvalidEdit, "%s", err.Error())
	}
	return result, nil
}

// Event reads persisted details and source suggestions without a network request.
func (s *Service) Event(ctx context.Context, id string) (Event, error) {
	return s.repo.LoadCatalogueEvent(ctx, id)
}

// Edit applies a revision-checked protected correction from an authenticated caller.
func (s *Service) Edit(ctx context.Context, id string, edit EventEdit, actor string) (Event, error) {
	return s.repo.EditCatalogueEvent(ctx, id, edit, actor)
}
