// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package update

import (
	"context"
	"testing"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/product"
	"github.com/autobrr/autobrr/pkg/version"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type forbiddenChecker struct{ t *testing.T }

func (c forbiddenChecker) CheckNewVersion(context.Context, string) (bool, *version.Release, error) {
	c.t.Fatal("UFC updater contacted a release source")
	return false, nil, nil
}

func TestUFCUpdatesDisabled(t *testing.T) {
	service := NewUpdate(zerolog.Nop(), nil, &domain.Config{Product: product.UFCManager(), CheckForUpdates: true})
	service.releaseChecker = forbiddenChecker{t}
	release, err := service.CheckUpdateAvailable(context.Background())
	require.NoError(t, err)
	require.Nil(t, release)
	service.CheckUpdates(context.Background())
	require.Nil(t, service.GetLatestRelease(context.Background()))
}
