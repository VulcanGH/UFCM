// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package processing

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/autobrr/autobrr/internal/ufc/acquisition"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/library"
	"github.com/stretchr/testify/require"
)

type copyRepo struct {
	previewRepo
	checkpoints []string
	completed   bool
}

type recoveryRepo struct {
	*copyRepo
	stored Job
}

func (r *recoveryRepo) EnqueueProcessingJob(_ context.Context, job Job) (Job, error) {
	r.stored = job
	return job, nil
}

func (r *recoveryRepo) ProcessingJob(context.Context, string) (Job, error) { return r.stored, nil }

func (r *recoveryRepo) SetProcessingDestination(_ context.Context, _, destination string) error {
	r.stored.Files[0].Destination = destination
	r.stored.State = "queued"
	return nil
}

func (r *copyRepo) SetProcessingFile(_ context.Context, _, _, state, _, _ string) error {
	r.checkpoints = append(r.checkpoints, state)
	return nil
}

func (r *copyRepo) SetProcessingProgress(context.Context, string, string, int64) error { return nil }

func (r *copyRepo) CompleteProcessingJob(_ context.Context, _ Job, _ int64, _, hash string) error {
	r.completed = hash != ""
	return nil
}

func TestCopyPreservesSourceAndNeverReplacesExistingMedia(t *testing.T) {
	base := t.TempDir()
	sourceRoot := filepath.Join(base, "downloads")
	storage := filepath.Join(sourceRoot, "SAB-owned")
	libraryRoot := filepath.Join(base, "library")
	require.NoError(t, os.MkdirAll(storage, 0700))
	require.NoError(t, os.MkdirAll(libraryRoot, 0700))
	sourceFile := filepath.Join(storage, "UFC.999.1080p.mkv")
	content := []byte("temporary media fixture")
	require.NoError(t, os.WriteFile(sourceFile, content, 0600))
	date := "2026-10-10"
	repo := &copyRepo{previewRepo: previewRepo{
		job:      acquisition.Job{ID: "owned", EventID: "event-999", Component: "main_card", State: "completed", RemoteID: "sab-1", Storage: storage},
		event:    catalogue.Event{ID: "event-999", Values: catalogue.Values{Name: "UFC 999", Type: "numbered", Date: &date, Headliners: []string{"Avery", "Blake"}, Status: "completed"}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "999", Current: true, Verified: true}}},
		settings: Settings{Revision: 1, SourceRoots: []string{sourceRoot}, DestinationRoot: libraryRoot, EventFolderTemplate: "{{.EventTitle}} ({{.Year}})", MainFileTemplate: "{{.EventTitle}} ({{.Year}})", ExtraFileTemplate: "{{.EventTitle}} ({{.Year}}) - {{.Component}}", ComponentFolder: "Featurettes", ConflictPolicy: "review", Extensions: []string{".mkv"}, MaxConcurrent: 1, MaxAttempts: 3},
		view:     library.View{Settings: &library.Settings{Roots: []library.Root{{Path: libraryRoot, Enabled: true, State: "available"}}}},
	}}
	service := New(repo, nil)
	plan, err := service.Preview(t.Context(), "owned")
	require.NoError(t, err)
	require.Equal(t, "ready", plan.Status)
	file := plan.Files[0]
	job := Job{ID: "fixture-job", AcquisitionID: "owned", EventID: "event-999", Component: "main_card", DestinationRoot: libraryRoot, Files: []JobFile{{Source: file.Source, Destination: file.Destination, Stage: filepath.ToSlash(filepath.Join(filepath.Dir(file.Destination), ".ufcm-fixture-job.part")), Size: file.Size}}}
	require.Empty(t, service.copyJob(t.Context(), job))
	require.True(t, repo.completed)
	require.Equal(t, []string{"staging", "published"}, repo.checkpoints)
	final := filepath.Join(libraryRoot, filepath.FromSlash(file.Destination))
	got, err := os.ReadFile(final)
	require.NoError(t, err)
	require.Equal(t, content, got)
	require.FileExists(t, sourceFile)
	require.NoFileExists(t, filepath.Join(libraryRoot, filepath.FromSlash(job.Files[0].Stage)))

	repo.completed = false
	repo.checkpoints = nil
	require.NotEmpty(t, service.copyJob(t.Context(), job))
	got, err = os.ReadFile(final)
	require.NoError(t, err)
	require.Equal(t, content, got)
	require.False(t, repo.completed)
	require.Empty(t, repo.checkpoints)
}

func TestKeepBothRequiresReviewAndPreservesConflict(t *testing.T) {
	base := t.TempDir()
	sourceRoot := filepath.Join(base, "downloads")
	storage := filepath.Join(sourceRoot, "SAB-owned")
	libraryRoot := filepath.Join(base, "library")
	require.NoError(t, os.MkdirAll(storage, 0700))
	require.NoError(t, os.MkdirAll(libraryRoot, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(storage, "UFC.999.1080p.mkv"), []byte("new content"), 0600))
	date := "2026-10-10"
	repo := &recoveryRepo{copyRepo: &copyRepo{previewRepo: previewRepo{
		job:      acquisition.Job{ID: "owned", EventID: "event-999", Component: "main_card", State: "completed", RemoteID: "sab-1", Storage: storage},
		event:    catalogue.Event{ID: "event-999", Values: catalogue.Values{Name: "UFC 999", Type: "numbered", Date: &date, Headliners: []string{"Avery", "Blake"}, Status: "completed"}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "999", Current: true, Verified: true}}},
		settings: Settings{Revision: 1, SourceRoots: []string{sourceRoot}, DestinationRoot: libraryRoot, EventFolderTemplate: "{{.EventTitle}} ({{.Year}})", MainFileTemplate: "{{.EventTitle}} ({{.Year}})", ExtraFileTemplate: "{{.EventTitle}} ({{.Year}}) - {{.Component}}", ComponentFolder: "Featurettes", ConflictPolicy: "review", Extensions: []string{".mkv"}, MaxConcurrent: 1, MaxAttempts: 3},
		view:     library.View{Settings: &library.Settings{Roots: []library.Root{{Path: libraryRoot, Enabled: true, State: "available"}}}},
	}}}
	service := New(repo, nil)
	plan, err := service.Preview(t.Context(), "owned")
	require.NoError(t, err)
	original := filepath.Join(libraryRoot, filepath.FromSlash(plan.Files[0].Destination))
	require.NoError(t, os.MkdirAll(filepath.Dir(original), 0700))
	require.NoError(t, os.WriteFile(original, []byte("existing content"), 0600))
	job, err := service.Queue(t.Context(), "owned")
	require.NoError(t, err)
	require.Equal(t, "needs_review", job.State)
	options, err := service.Review(t.Context(), job.ID)
	require.NoError(t, err)
	require.True(t, options.Skip)
	require.Contains(t, options.KeepBothDestination, "(copy 2)")
	job, err = service.KeepBoth(t.Context(), job.ID, options.KeepBothDestination)
	require.NoError(t, err)
	require.Equal(t, "queued", job.State)
	require.Empty(t, service.copyJob(t.Context(), job))
	before, err := os.ReadFile(original)
	require.NoError(t, err)
	require.Equal(t, []byte("existing content"), before)
	separate, err := os.ReadFile(filepath.Join(libraryRoot, filepath.FromSlash(options.KeepBothDestination)))
	require.NoError(t, err)
	require.Equal(t, []byte("new content"), separate)
}

func TestRestartReconcilesOnlyPublishedHashEvidence(t *testing.T) {
	root := t.TempDir()
	relative := "UFC 999/UFC 999.mkv"
	final := filepath.Join(root, filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(final), 0700))
	content := []byte("verified copy")
	require.NoError(t, os.WriteFile(final, content, 0600))
	sum := sha256.Sum256(content)
	repo := &copyRepo{}
	service := New(repo, nil)
	job := Job{ID: "known-job", State: "needs_review", DestinationRoot: root, Files: []JobFile{{Source: "source.mkv", Destination: relative, Stage: "UFC 999/.ufcm-known-job.part", Size: int64(len(content)), State: "published", SourceSHA256: hex.EncodeToString(sum[:])}}}
	require.True(t, service.reconcileOne(t.Context(), job))
	require.True(t, repo.completed)
	repo.completed = false
	job.Files[0].State = "staging"
	require.False(t, service.reconcileOne(t.Context(), job), "a staging checkpoint alone does not prove publication")
	require.False(t, repo.completed)
	require.FileExists(t, final)
}

func TestMatchedMultiFileBundleGetsDistinctNames(t *testing.T) {
	base := t.TempDir()
	sourceRoot := filepath.Join(base, "downloads")
	storage := filepath.Join(sourceRoot, "SAB-owned")
	libraryRoot := filepath.Join(base, "library")
	require.NoError(t, os.MkdirAll(storage, 0700))
	require.NoError(t, os.MkdirAll(libraryRoot, 0700))
	for _, name := range []string{"UFC.999.Main.Card.Part1.1080p.mkv", "UFC.999.Main.Card.Part2.1080p.mkv", "UFC.999.Prelims.1080p.mkv"} {
		require.NoError(t, os.WriteFile(filepath.Join(storage, name), []byte(name), 0600))
	}
	date := "2026-10-10"
	repo := &copyRepo{previewRepo: previewRepo{
		job:      acquisition.Job{ID: "owned", EventID: "event-999", Component: "main_card", State: "completed", RemoteID: "sab-1", Storage: storage},
		event:    catalogue.Event{ID: "event-999", Values: catalogue.Values{Name: "UFC 999", Type: "numbered", Date: &date, Headliners: []string{"Avery", "Blake"}, Status: "completed"}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "999", Current: true, Verified: true}}},
		settings: Settings{Revision: 1, SourceRoots: []string{sourceRoot}, DestinationRoot: libraryRoot, EventFolderTemplate: "{{.EventTitle}} ({{.Year}})", MainFileTemplate: "{{.EventTitle}} ({{.Year}})", ExtraFileTemplate: "{{.EventTitle}} ({{.Year}}) - {{.Component}}", ComponentFolder: "Featurettes", ConflictPolicy: "review", Extensions: []string{".mkv"}, MaxConcurrent: 1, MaxAttempts: 3},
		view:     library.View{Settings: &library.Settings{Roots: []library.Root{{Path: libraryRoot, Enabled: true, State: "available"}}}},
	}}
	service := New(repo, nil)
	plan, err := service.Preview(t.Context(), "owned")
	require.NoError(t, err)
	require.Equal(t, "ready", plan.Status)
	require.Len(t, plan.Files, 3)
	require.Equal(t, []string{"main_card", "main_card", "prelims"}, []string{plan.Files[0].Component, plan.Files[1].Component, plan.Files[2].Component})
	require.Contains(t, plan.Files[0].Destination, "Part 1")
	require.Contains(t, plan.Files[1].Destination, "Part 2")
	require.Contains(t, plan.Files[2].Destination, "Featurettes")
	job := Job{ID: "multi", AcquisitionID: "owned", EventID: "event-999", Component: "main_card", DestinationRoot: libraryRoot}
	for index, file := range plan.Files {
		job.Files = append(job.Files, JobFile{Source: file.Source, Component: file.Component, Destination: file.Destination, OriginalDestination: file.Destination, Stage: filepath.ToSlash(filepath.Join(filepath.Dir(file.Destination), ".ufcm-multi-"+strconv.Itoa(index+1)+".part")), Size: file.Size})
	}
	require.Empty(t, service.copyJob(t.Context(), job))
	for _, file := range job.Files {
		require.FileExists(t, filepath.Join(libraryRoot, filepath.FromSlash(file.Destination)))
		require.FileExists(t, filepath.Join(storage, filepath.FromSlash(file.Source)))
	}
}
