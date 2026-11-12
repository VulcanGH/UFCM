// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package acquisition

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/pkg/errors"
)

func recoveryTitle(value, marker string) string {
	value = strings.TrimSpace(value)
	if strings.HasSuffix(strings.ToLower(value), ".nzb") {
		value = value[:len(value)-4]
	}
	value = strings.TrimPrefix(value, marker+" ")
	value = strings.TrimSuffix(value, " "+marker)
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, value)), " ")
}

// Recover verifies and binds an existing SAB job without resubmission or media access.
func (s *Service) Recover(ctx context.Context, id string, input Recovery) (*Job, error) {
	if len(id) == 0 || len(id) > 128 || !input.Confirm || input.Revision < 1 || len(input.RemoteID) == 0 || len(input.RemoteID) > 256 || strings.IndexFunc(input.RemoteID, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-')
	}) >= 0 {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "confirm a valid SAB job ID and current download revision")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	j, err := s.repo.Acquisition(ctx, id)
	if err != nil {
		return nil, err
	}
	if j.Revision != input.Revision {
		return nil, catalogue.ErrRevisionConflict
	}
	if j.State == "completed" || j.State == "failed" {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "terminal downloads cannot be rebound; confirmed failures require a fresh search")
	}
	if j.RemoteID != "" && j.RemoteID != input.RemoteID {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "this download already has a different SAB ID; existing bindings cannot be replaced")
	}
	target, client, err := s.gateway.Target(ctx, j.ClientID)
	if err != nil || target.Endpoint != j.Endpoint {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "saved SAB endpoint is unavailable or changed")
	}
	remote, err := client.Lookup(ctx, input.RemoteID, "")
	if err != nil {
		return nil, errors.New("could not verify the SAB job; no binding or submission was made")
	}
	if !remote.Found || remote.Ambiguous || remote.ID != input.RemoteID || recoveryTitle(remote.Title, "UFCM-"+j.ID) != recoveryTitle(j.Title, "") {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "SAB job is missing, ambiguous or its title differs from this release; nothing was changed")
	}
	previous := j.RemoteID
	j = applyRemote(j, remote)
	if err := s.repo.BindAcquisition(ctx, j, input.Actor, previous); err != nil {
		return nil, err
	}
	j.Revision++
	j.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return &j, nil
}
