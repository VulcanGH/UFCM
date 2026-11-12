// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package processing

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/autobrr/autobrr/internal/ufc/acquisition"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/library"
	"github.com/stretchr/testify/require"
)

type handoffRepo struct{ *recoveryRepo }

func (r *handoffRepo) FinishProcessingHandoff(context.Context, string) (bool, error) {
	return false, nil
}

func TestCompletionHandoffRequiresOptInAndMatchingSAB(t *testing.T) {
	base := t.TempDir()
	sourceRoot, libraryRoot := filepath.Join(base, "downloads"), filepath.Join(base, "library")
	storage := filepath.Join(sourceRoot, "owned-job")
	require.NoError(t, os.MkdirAll(storage, 0700))
	require.NoError(t, os.MkdirAll(libraryRoot, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(storage, "UFC.999.1080p.mkv"), []byte("temporary fixture"), 0600))
	date := "2026-10-10"
	download := acquisition.Job{ID: "owned", EventID: "event-999", Component: "main_card", State: "completed", RemoteID: "sab-1", Storage: storage, ClientID: 4, Category: "ufc"}
	repo := &recoveryRepo{copyRepo: &copyRepo{previewRepo: previewRepo{
		job:      download,
		event:    catalogue.Event{ID: "event-999", Values: catalogue.Values{Name: "UFC 999", Type: "numbered", Date: &date, Headliners: []string{"Avery", "Blake"}, Status: "completed"}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "999", Current: true, Verified: true}}},
		settings: Settings{Revision: 1, SourceRoots: []string{sourceRoot}, DestinationRoot: libraryRoot, EventFolderTemplate: "{{.EventTitle}} ({{.Year}})", MainFileTemplate: "{{.EventTitle}} ({{.Year}})", ExtraFileTemplate: "{{.EventTitle}} ({{.Year}}) - {{.Component}}", ComponentFolder: "Featurettes", ConflictPolicy: "review", Extensions: []string{".mkv"}, MaxConcurrent: 1, MaxAttempts: 3, SABClientID: 4, SABCategory: "ufc"},
		view:     library.View{Settings: &library.Settings{Roots: []library.Root{{Path: libraryRoot, Enabled: true, State: "available"}}}, Groups: []library.Group{{EventID: "event-999", Files: 1, Locations: 1, Components: map[string]string{"prelims": "present", "main_card": "missing"}}}},
	}}}
	service := New(&handoffRepo{repo}, nil)
	require.NoError(t, service.CompletedDownload(t.Context(), download))
	require.Empty(t, repo.stored.ID)
	repo.settings.AutoProcessCompleted = true
	wrong := download
	wrong.ClientID = 5
	require.NoError(t, service.CompletedDownload(t.Context(), wrong))
	require.Empty(t, repo.stored.ID)
	require.NoError(t, service.CompletedDownload(t.Context(), download))
	require.Equal(t, "owned", repo.stored.AcquisitionID)
	require.Equal(t, "queued", repo.stored.State)
	require.FileExists(t, filepath.Join(storage, "UFC.999.1080p.mkv"))
	repo.view.Groups[0].Components["main_card"] = "present"
	plan, err := service.Preview(t.Context(), download.ID)
	require.NoError(t, err)
	require.Equal(t, "needs_review", plan.Status)
}
