// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

//go:build integration

package database

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/acquisition"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/stretchr/testify/require"
)

func checkManualWorkflow(t *testing.T, r *Repository) {
	ctx := t.Context()
	before, err := r.CataloguePreferences(ctx)
	require.NoError(t, err)
	require.Nil(t, before.MainCardTerms)
	edit := catalogue.SettingsEdit{Revision: before.Revision, Templates: before.Templates, MainCardTerms: json.RawMessage(`["pre show","special show"]`)}
	custom, err := r.SaveCataloguePreferences(ctx, edit)
	require.NoError(t, err)
	require.Equal(t, []string{"pre show", "special show"}, custom.MainCardTerms)
	_, err = r.SaveCataloguePreferences(ctx, edit)
	require.ErrorIs(t, err, catalogue.ErrRevisionConflict)
	edit.Revision = custom.Revision
	edit.MainCardTerms = json.RawMessage(`["(pre.*show)"]`)
	_, err = r.SaveCataloguePreferences(ctx, edit)
	require.Error(t, err)
	edit.MainCardTerms = json.RawMessage(`[]`)
	empty, err := r.SaveCataloguePreferences(ctx, edit)
	require.NoError(t, err)
	require.NotNil(t, empty.MainCardTerms)
	require.Empty(t, empty.MainCardTerms)
	edit.Revision = empty.Revision
	edit.MainCardTerms = json.RawMessage(`null`)
	restored, err := r.SaveCataloguePreferences(ctx, edit)
	require.NoError(t, err)
	require.Nil(t, restored.MainCardTerms)
	require.Equal(t, before.Templates, restored.Templates)
	require.Equal(t, before.Resolutions, restored.Resolutions)
	event, err := r.CreateCatalogueEvent(ctx, catalogue.EventCreate{Values: catalogue.Values{Name: "Workflow fixture", Type: "numbered", Status: "completed"}}, "test")
	require.NoError(t, err)
	job := acquisition.Job{ID: "manual-audit", EventID: event.ID, EventRevision: event.Revision, SettingsRevision: event.Quality.SettingsRevision, Component: "main_card", Title: "Fixture.1080p", Source: "fake", ClientID: 1, ClientName: "fake", Endpoint: "fake-endpoint", Actor: "user:test", Key: "manual-audit-key", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), Decision: &acquisition.Decision{Manual: true, Actor: "user:test", Reason: "Reviewed fixture"}}
	saved, created, err := r.ReserveAcquisition(ctx, job)
	require.NoError(t, err)
	require.True(t, created)
	retained, err := r.Acquisition(ctx, job.ID)
	require.NoError(t, err)
	require.Equal(t, job.Decision, retained.Decision)
	saved.RemoteID = "sab-bound"
	saved.State = "queued"
	require.NoError(t, r.BindAcquisition(ctx, saved, "user:reviewer", ""))
	require.ErrorIs(t, r.BindAcquisition(ctx, saved, "user:stale", ""), catalogue.ErrRevisionConflict)
	var count int
	require.NoError(t, r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ufc_acquisition_recovery WHERE acquisition_id=$1 AND actor='user:reviewer' AND previous_remote_id='' AND remote_id='sab-bound'", job.ID).Scan(&count))
	require.Equal(t, 1, count)
	job.ID = "manual-second"
	job.Key = "manual-second-key"
	job.Component = "prelims"
	second, created, err := r.ReserveAcquisition(ctx, job)
	require.NoError(t, err)
	require.True(t, created)
	second.RemoteID = "sab-bound"
	second.State = "queued"
	require.Error(t, r.BindAcquisition(ctx, second, "user:test", ""))
	retained, err = r.Acquisition(ctx, second.ID)
	require.NoError(t, err)
	require.Empty(t, retained.RemoteID)
	require.Equal(t, second.Revision, retained.Revision)
	require.NoError(t, r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ufc_acquisition_recovery WHERE acquisition_id=$1", second.ID).Scan(&count))
	require.Zero(t, count)
}
