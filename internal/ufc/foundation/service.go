// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package foundation reports the available UFC application capabilities.
package foundation

import (
	"context"

	"github.com/autobrr/autobrr/internal/product"
	"github.com/autobrr/autobrr/pkg/errors"
)

// ApplicationSettings contains persistent Foundation capability preferences.
type ApplicationSettings struct {
	Revision                    int
	AutomaticAcquisitionEnabled bool
	ProcessingEnabled           bool
}

// Status describes the product and capabilities actually available in this build.
type Status struct {
	ProductSlug                 string `json:"product_slug"`
	ProductName                 string `json:"product_name"`
	SchemaReady                 bool   `json:"schema_ready"`
	SettingsRevision            int    `json:"settings_revision"`
	AutomaticAcquisitionEnabled bool   `json:"automatic_acquisition_enabled"`
	ProcessingEnabled           bool   `json:"processing_enabled"`
}

type settingsRepository interface {
	LoadApplicationSettings(context.Context) (ApplicationSettings, error)
}

// Service reads Foundation status without starting acquisition or processing.
type Service struct {
	repo     settingsRepository
	identity product.Identity
}

// New returns a read-only Foundation service.
func New(repo settingsRepository, identity product.Identity) *Service {
	return &Service{repo: repo, identity: identity}
}

// Status verifies persistent settings before reporting disabled capabilities.
func (s *Service) Status(ctx context.Context) (*Status, error) {
	settings, err := s.repo.LoadApplicationSettings(ctx)
	if err != nil {
		return nil, err
	}
	if settings.Revision < 1 || settings.ProcessingEnabled {
		return nil, errors.New("unsupported UFC Foundation capability settings")
	}
	return &Status{ProductSlug: s.identity.Slug(), ProductName: s.identity.Name(), SchemaReady: true, SettingsRevision: settings.Revision, AutomaticAcquisitionEnabled: settings.AutomaticAcquisitionEnabled}, nil
}
