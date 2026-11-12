// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package processing

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/downloader"
	"github.com/autobrr/autobrr/internal/ufc/acquisition"
	"github.com/autobrr/autobrr/internal/ufc/library"
	"github.com/autobrr/autobrr/pkg/sabnzbd"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

type cleanupDownloaders struct{ instance *downloader.Instance }

func (d cleanupDownloaders) GetInstance(context.Context, int32) (*downloader.Instance, error) {
	return d.instance, nil
}

func TestCleanupUsesExactSABHistoryBindingWhenStorageWasRenamed(t *testing.T) {
	for _, scenario := range []string{"owned", "different storage", "different ID", "missing marker", "not completed", "ambiguous"} {
		t.Run(scenario, func(t *testing.T) {
			service, job, storage, libraryRoot := cleanupFixture(t, "Renamed release", "card.mkv")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("mode") == "queue" {
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"queue": map[string]any{"slots": []any{}}}))
					return
				}
				require.Equal(t, "sab-job", r.URL.Query().Get("nzo_ids"))
				slot := map[string]string{"nzo_id": "sab-job", "name": "Release UFCM-owned.nzb", "status": "Completed", "storage": storage}
				switch scenario {
				case "different storage":
					slot["storage"] = filepath.Join(filepath.Dir(storage), "Other release")
				case "different ID":
					slot["nzo_id"] = "other-job"
				case "missing marker":
					slot["name"] = "Release"
				case "not completed":
					slot["status"] = "Failed"
				}
				slots := []map[string]string{slot}
				if scenario == "ambiguous" {
					slots = append(slots, slot)
				}
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"history": map[string]any{"slots": slots}}))
			}))
			defer server.Close()
			client := sabnzbd.New(sabnzbd.Options{Addr: server.URL, ApiKey: "fixture", Log: zerolog.Nop()})
			service.downloaders = cleanupDownloaders{downloader.NewInstance(&domain.Downloader{Enabled: true, Type: domain.DownloaderTypeSabnzbd}, client)}
			message := service.cleanupCompleted(t.Context(), job)
			if scenario == "owned" {
				require.Empty(t, message)
				require.NoDirExists(t, storage)
			} else {
				require.NotEmpty(t, message)
				require.FileExists(t, filepath.Join(storage, "card.mkv"))
			}
			require.FileExists(t, filepath.Join(libraryRoot, "event.mkv"))
		})
	}
}

func cleanupFixture(t *testing.T, relative, filename string) (*Service, Job, string, string) {
	t.Helper()
	base := t.TempDir()
	sourceRoot, libraryRoot := filepath.Join(base, "downloads"), filepath.Join(base, "library")
	storage := filepath.Join(sourceRoot, relative)
	require.NoError(t, os.MkdirAll(storage, 0700))
	require.NoError(t, os.MkdirAll(libraryRoot, 0700))
	content := []byte("temporary cleanup fixture")
	source := filepath.Join(storage, filename)
	require.NoError(t, os.WriteFile(source, content, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(libraryRoot, "event.mkv"), content, 0600))
	hash, _, err := hashFile(t.Context(), source)
	require.NoError(t, err)
	repo := previewRepo{
		job:      acquisition.Job{ID: "owned", EventID: "event", State: "completed", RemoteID: "sab-job", Storage: storage, ClientID: 4, Category: "ufc"},
		settings: Settings{SourceRoots: []string{sourceRoot}, DestinationRoot: libraryRoot, SABClientID: 4, SABCategory: "ufc", CleanupSABDownload: true},
		view:     library.View{Settings: &library.Settings{Roots: []library.Root{{Path: libraryRoot, Enabled: true}}}},
	}
	job := Job{ID: "copy", AcquisitionID: "owned", EventID: "event", DestinationRoot: libraryRoot, State: "completed", CleanupRequested: true, Files: []JobFile{{Source: filename, Destination: "event.mkv", State: "verified", Size: int64(len(content)), SourceSHA256: hash, DestinationSHA256: hash}}}
	return New(repo, nil), job, storage, libraryRoot
}

func TestCleanupRetryFinishesPartialDirectory(t *testing.T) {
	service, job, storage, libraryRoot := cleanupFixture(t, "UFCM-owned", "card.mkv")
	content, err := os.ReadFile(filepath.Join(storage, "card.mkv"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(storage, "prelims.mkv"), content, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(libraryRoot, "prelims.mkv"), content, 0600))
	second := job.Files[0]
	second.Source, second.Destination = "prelims.mkv", "prelims.mkv"
	job.Files = append(job.Files, second)
	require.NoError(t, os.Remove(filepath.Join(storage, "card.mkv")))
	require.Empty(t, service.cleanupCompleted(t.Context(), job))
	require.NoDirExists(t, storage)
	require.FileExists(t, filepath.Join(libraryRoot, "event.mkv"))
	require.FileExists(t, filepath.Join(libraryRoot, "prelims.mkv"))
	require.Empty(t, service.cleanupCompleted(t.Context(), job))
}

func TestCleanupAcceptsNestedStorageWithOwnedFilename(t *testing.T) {
	service, job, storage, libraryRoot := cleanupFixture(t, filepath.Join("ufc", "Completed release"), "card UFCM-owned.mkv")
	require.Empty(t, service.cleanupCompleted(t.Context(), job))
	require.NoDirExists(t, storage)
	require.DirExists(t, filepath.Dir(storage))
	require.FileExists(t, filepath.Join(libraryRoot, "event.mkv"))
}

func TestCleanupRetainsSourceWhenSameSizeLibraryContentChanged(t *testing.T) {
	service, job, storage, libraryRoot := cleanupFixture(t, "UFCM-owned", "card.mkv")
	require.NoError(t, os.WriteFile(filepath.Join(libraryRoot, "event.mkv"), bytes.Repeat([]byte("x"), int(job.Files[0].Size)), 0600))
	require.NotEmpty(t, service.cleanupCompleted(t.Context(), job))
	require.FileExists(t, filepath.Join(storage, "card.mkv"))
}

func TestCleanupRejectsSourceRootAndDifferentMarker(t *testing.T) {
	service, job, storage, _ := cleanupFixture(t, "UFCM-ownedOTHER", "card.mkv")
	require.NotEmpty(t, service.cleanupCompleted(t.Context(), job))
	require.FileExists(t, filepath.Join(storage, "card.mkv"))
	service, job, storage, _ = cleanupFixture(t, "UFCM-owned", "card.mkv")
	repo := service.repo.(previewRepo)
	repo.settings.SourceRoots = append(repo.settings.SourceRoots, storage)
	service.repo = repo
	require.NotEmpty(t, service.cleanupCompleted(t.Context(), job))
	require.FileExists(t, filepath.Join(storage, "card.mkv"))
}

func TestCleanupRequiresExactVerifiedCompletedStorage(t *testing.T) {
	base := t.TempDir()
	sourceRoot, libraryRoot := filepath.Join(base, "downloads"), filepath.Join(base, "library")
	storage := filepath.Join(sourceRoot, "UFCM-owned")
	require.NoError(t, os.MkdirAll(storage, 0700))
	require.NoError(t, os.MkdirAll(libraryRoot, 0700))
	source, destination := filepath.Join(storage, "card.mkv"), filepath.Join(libraryRoot, "event.mkv")
	content := []byte("temporary fixture")
	require.NoError(t, os.WriteFile(source, content, 0600))
	require.NoError(t, os.WriteFile(destination, content, 0600))
	hash, _, err := hashFile(t.Context(), source)
	require.NoError(t, err)
	repo := previewRepo{
		job:      acquisition.Job{ID: "owned", EventID: "event", State: "completed", RemoteID: "sab-job", Storage: storage, ClientID: 4, Category: "ufc"},
		settings: Settings{SourceRoots: []string{sourceRoot}, DestinationRoot: libraryRoot, SABClientID: 4, SABCategory: "ufc", CleanupSABDownload: true},
		view:     library.View{Settings: &library.Settings{Roots: []library.Root{{Path: libraryRoot, Enabled: true}}}},
	}
	service := New(repo, nil)
	job := Job{ID: "copy", AcquisitionID: "owned", EventID: "event", DestinationRoot: libraryRoot, State: "completed", CleanupRequested: true, Files: []JobFile{{Source: "card.mkv", Destination: "event.mkv", State: "verified", Size: int64(len(content)), SourceSHA256: hash, DestinationSHA256: hash}}}
	repo.settings.CleanupSABDownload = false
	service.repo = repo
	require.NotEmpty(t, service.cleanupCompleted(t.Context(), job))
	require.FileExists(t, source)
	repo.settings.CleanupSABDownload = true
	repo.job.ID = "different-job"
	service.repo = repo
	require.NotEmpty(t, service.cleanupCompleted(t.Context(), job))
	require.FileExists(t, source)
	repo.job.ID = "owned"
	service.repo = repo
	extra := filepath.Join(storage, "unverified.mkv")
	require.NoError(t, os.WriteFile(extra, []byte("unplanned media"), 0600))
	job.Files[0].DestinationSHA256 = "changed"
	require.NotEmpty(t, service.cleanupCompleted(t.Context(), job))
	require.FileExists(t, source)
	require.FileExists(t, extra)
	job.Files[0].DestinationSHA256 = hash
	require.NoError(t, os.WriteFile(destination, []byte("short"), 0600))
	require.NotEmpty(t, service.cleanupCompleted(t.Context(), job))
	require.FileExists(t, source)
	require.NoError(t, os.WriteFile(destination, content, 0600))
	sidecar := filepath.Join(storage, "release.nfo")
	require.NoError(t, os.WriteFile(sidecar, []byte("release notes"), 0600))
	require.Empty(t, service.cleanupCompleted(t.Context(), job))
	require.NoDirExists(t, storage)
	require.NoFileExists(t, extra)
	require.FileExists(t, destination)
	require.Empty(t, service.cleanupCompleted(t.Context(), job))
	require.NoError(t, os.WriteFile(destination, []byte("short"), 0600))
	require.NotEmpty(t, service.cleanupCompleted(t.Context(), job))
}
