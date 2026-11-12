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

type previewRepo struct {
	repository
	job      acquisition.Job
	event    catalogue.Event
	settings Settings
	view     library.View
}

func (r previewRepo) Acquisition(context.Context, string) (acquisition.Job, error) { return r.job, nil }
func (r previewRepo) LoadCatalogueEvent(context.Context, string) (catalogue.Event, error) {
	return r.event, nil
}
func (r previewRepo) LibraryView(context.Context) (*library.View, error) { return &r.view, nil }
func (r previewRepo) CataloguePreferences(context.Context) (*catalogue.Settings, error) {
	return &catalogue.Settings{}, nil
}
func (r previewRepo) ProcessingSettings(context.Context) (*Settings, error) { return &r.settings, nil }
func (r previewRepo) SaveProcessingSettings(_ context.Context, s Settings) (*Settings, error) {
	return &s, nil
}
func (r previewRepo) LibrarySettings(context.Context) (*library.Settings, error) {
	return r.view.Settings, nil
}

func TestPreviewUsesCatalogueNameWithoutWriting(t *testing.T) {
	base := t.TempDir()
	source := filepath.Join(base, "downloads")
	destination := filepath.Join(base, "library")
	storage := filepath.Join(source, "SAB-owned")
	require.NoError(t, os.MkdirAll(storage, 0700))
	require.NoError(t, os.MkdirAll(destination, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(storage, "UFC.999.1080p.mkv"), []byte("temporary fixture"), 0600))
	date := "2026-10-10"
	repo := previewRepo{
		job:      acquisition.Job{ID: "owned", EventID: "event-999", Component: "main_card", State: "completed", RemoteID: "sab-1", Storage: storage},
		event:    catalogue.Event{ID: "event-999", Values: catalogue.Values{Name: "UFC 999", Type: "numbered", Date: &date, Headliners: []string{"Avery", "Blake"}, Status: "completed"}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "999", Current: true, Verified: true}}},
		settings: Settings{Revision: 1, SourceRoots: []string{source}, DestinationRoot: destination, EventFolderTemplate: "{{.EventTitle}} ({{.Year}})", MainFileTemplate: "{{.EventTitle}} ({{.Year}})", ExtraFileTemplate: "{{.EventTitle}} ({{.Year}}) - {{.Component}}", ComponentFolder: "Featurettes", ConflictPolicy: "review", Extensions: []string{".mkv"}, MaxConcurrent: 1, MaxAttempts: 3},
		view:     library.View{Settings: &library.Settings{Roots: []library.Root{{Path: destination, Enabled: true, State: "available"}}}, Groups: []library.Group{}},
	}
	service := New(repo, nil)
	preview, err := service.Preview(t.Context(), "owned")
	require.NoError(t, err)
	require.Equal(t, "ready", preview.Status)
	require.Equal(t, "UFC 999 Avery vs. Blake", preview.EventTitle)
	require.Len(t, preview.Files, 1)
	require.Equal(t, "UFC 999 Avery vs. Blake (2026)/UFC 999 Avery vs. Blake (2026).mkv", preview.Files[0].Destination)
	_, err = os.Stat(filepath.Join(destination, filepath.FromSlash(preview.Files[0].Destination)))
	require.True(t, os.IsNotExist(err))
	require.FileExists(t, filepath.Join(storage, "UFC.999.1080p.mkv"))

	conflict := filepath.Join(destination, filepath.FromSlash(preview.Files[0].Destination))
	require.NoError(t, os.MkdirAll(filepath.Dir(conflict), 0700))
	require.NoError(t, os.WriteFile(conflict, []byte("different fixture"), 0600))
	preview, err = service.Preview(t.Context(), "owned")
	require.NoError(t, err)
	require.Equal(t, "needs_review", preview.Status)
	require.Equal(t, "review", preview.Files[0].State)
	require.FileExists(t, conflict)
}
