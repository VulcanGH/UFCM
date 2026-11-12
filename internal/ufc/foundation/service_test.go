// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package foundation

import (
	"context"
	"errors"
	"testing"

	"github.com/autobrr/autobrr/internal/product"

	"github.com/stretchr/testify/require"
)

type settingsStub struct {
	settings ApplicationSettings
	err      error
}

func (s settingsStub) LoadApplicationSettings(context.Context) (ApplicationSettings, error) {
	return s.settings, s.err
}
func TestStatus(t *testing.T) {
	service := New(settingsStub{settings: ApplicationSettings{Revision: 1}}, product.UFCManager())
	status, err := service.Status(t.Context())
	require.NoError(t, err)
	require.True(t, status.SchemaReady)
	require.False(t, status.AutomaticAcquisitionEnabled)
	require.False(t, status.ProcessingEnabled)
	for _, settings := range []ApplicationSettings{{}, {Revision: 1, ProcessingEnabled: true}} {
		_, err := New(settingsStub{settings: settings}, product.UFCManager()).Status(t.Context())
		require.Error(t, err)
	}
	enabled, err := New(settingsStub{settings: ApplicationSettings{Revision: 2, AutomaticAcquisitionEnabled: true}}, product.UFCManager()).Status(t.Context())
	require.NoError(t, err)
	require.True(t, enabled.AutomaticAcquisitionEnabled)
	failure := errors.New("database unavailable")
	_, err = New(settingsStub{err: failure}, product.UFCManager()).Status(t.Context())
	require.ErrorIs(t, err, failure)
}
