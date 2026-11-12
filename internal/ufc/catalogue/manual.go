// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/autobrr/autobrr/pkg/errors"
)

// AliasCorrection protects an explicit alias list, including an empty list.
type AliasCorrection struct {
	Protected bool      `json:"protected"`
	Values    []Alias   `json:"values"`
	Actor     string    `json:"actor"`
	At        time.Time `json:"at"`
}

// AliasEdit distinguishes a protected alias list from resetting to the source.
type AliasEdit struct {
	Operation string  `json:"operation"`
	Values    []Alias `json:"values,omitempty"`
}

// EventCreate contains local facts without a provider identity.
type EventCreate struct {
	Values  Values  `json:"values"`
	Aliases []Alias `json:"aliases"`
}

// ManualEvent protects all supplied facts without inventing a Wikipedia source.
func ManualEvent(input EventCreate, actor string, at time.Time) (Event, error) {
	if err := validateValues(input.Values); err != nil {
		return Event{}, errors.Wrap(ErrInvalidEdit, "%s", err.Error())
	}
	e := Event{Revision: 1, Values: input.Values, Review: []string{}}
	return ApplyEdit(e, EventEdit{Revision: 1,
		Name:       &FieldEdit{Operation: "set", Value: &input.Values.Name},
		Type:       &FieldEdit{Operation: "set", Value: &input.Values.Type},
		Date:       &FieldEdit{Operation: "set", Value: input.Values.Date},
		Headliners: &FieldEdit{Operation: "set", Names: input.Values.Headliners},
		Status:     &FieldEdit{Operation: "set", Value: &input.Values.Status},
		Aliases:    &AliasEdit{Operation: "set", Values: input.Aliases}}, actor, at)
}

func manualAliases(values []Alias) ([]Alias, error) {
	if len(values) > 96 {
		return nil, errors.Wrap(ErrInvalidEdit, "alias limit exceeded")
	}
	result := []Alias{}
	seen := map[string]bool{}
	for _, a := range values {
		a.Value = strings.TrimSpace(a.Value)
		a.Manual = true
		if err := ValidateAlias(a); err != nil {
			return nil, errors.Wrap(ErrInvalidEdit, "%s", err.Error())
		}
		if a.Namespace != "title" && a.Value != "37.5" {
			number, err := strconv.ParseUint(a.Value, 10, 32)
			if err != nil || number == 0 || strconv.FormatUint(number, 10) != a.Value {
				return nil, errors.Wrap(ErrInvalidEdit, "numbering aliases must be positive canonical numbers without leading zeroes")
			}
		}
		key := a.Namespace + ":" + Normalize(a.Value)
		if seen[key] {
			return nil, errors.Wrap(ErrInvalidEdit, "duplicate alias")
		}
		seen[key] = true
		result = append(result, a)
	}
	return result, nil
}

// Create saves a manually identified event with its own canonical local ID.
func (s *Service) Create(ctx context.Context, input EventCreate, actor string) (Event, error) {
	return s.repo.CreateCatalogueEvent(ctx, input, actor)
}
