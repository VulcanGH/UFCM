// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/autobrr/autobrr/internal/maintenance"

	"github.com/stretchr/testify/require"
)

func TestCheckFeedback(t *testing.T) {
	repo := &memoryRepo{state: State{Revision: 1, Phase: "idle"}}
	s := New(repo, "1.0.0", "sqlite", "", t.TempDir(), nil)
	s.client = nil
	_, err := s.Check(t.Context())
	require.ErrorContains(t, err, "signing key is not configured")
	client, _, _, _, _ := fixture(t, "")
	s.client = client
	status, err := s.Check(t.Context())
	require.NoError(t, err)
	require.Equal(t, "checking", status.Phase)
	for _, code := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		client.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
		})
		_, _, err = client.Latest(t.Context(), 0)
		require.Equal(t, code == http.StatusNotFound, errors.Is(err, errNoRelease))
		_, err = s.check(t.Context())
		require.NoError(t, err)
		status, err = s.Status(t.Context())
		require.NoError(t, err)
		if code == http.StatusNotFound {
			require.Equal(t, "no_release", status.Phase)
		} else {
			require.Equal(t, "check_failed", status.Phase)
		}
		require.NotEmpty(t, status.Message)
	}
}

func TestMissingSignedAssetIsNotNoPublicRelease(t *testing.T) {
	client, _, manifest, _, _ := fixture(t, "")
	client.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "api.github.com" {
			body, err := json.Marshal(map[string]any{"tag_name": manifest.Version})
			require.NoError(t, err)
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
		}
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	})
	_, _, err := client.Latest(t.Context(), 0)
	require.Error(t, err)
	require.NotErrorIs(t, err, errNoRelease)
}

func TestDiscoveryAuthenticatesGitHubMetadataAndIgnoresDrafts(t *testing.T) {
	client, envelope, manifest, _, _ := fixture(t, "")
	draft := false
	tamper := false
	client.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
		require.Empty(t, r.Header.Get("Authorization"))
		var body []byte
		switch {
		case r.URL.Host == "api.github.com":
			body, _ = json.Marshal(map[string]any{"tag_name": manifest.Version, "draft": draft, "prerelease": false})
		case strings.HasSuffix(r.URL.Path, "ufc-update.json"):
			body = append([]byte{}, envelope.Manifest...)
			if tamper {
				body = append(body, ' ')
			}
		case strings.HasSuffix(r.URL.Path, "ufc-update.sig"):
			body = envelope.Signature
		default:
			t.Fatalf("unexpected release request: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
	})
	received, _, err := client.Latest(t.Context(), 0)
	require.NoError(t, err)
	require.Equal(t, ID(envelope), ID(received))
	draft = true
	_, _, err = client.Latest(t.Context(), 0)
	require.Error(t, err)
	draft = false
	tamper = true
	_, _, err = client.Latest(t.Context(), 0)
	require.ErrorContains(t, err, "signature")
}

func TestBackupDeferralPreservesPreferenceWithoutRestartLoop(t *testing.T) {
	root := t.TempDir()
	repo := &memoryRepo{state: State{Revision: 1, AutoInstall: true, Phase: "quiescing", ID: "release", Version: "v1.0.1"}}
	require.NoError(t, WriteJSON(filepath.Join(root, "bootstrap-state.json"), Bootstrap{Active: "seed", DeferredID: "release"}))
	service := New(repo, "1.0.0", "sqlite", "unused.db", "profile", nil)
	service.root = root
	require.NoError(t, service.Start(t.Context()))
	defer func() { require.NoError(t, service.Stop(context.Background())) }()
	status, err := service.Status(t.Context())
	require.NoError(t, err)
	require.Equal(t, "deferred", status.Phase)
	require.True(t, status.AutoInstall)
	require.Empty(t, status.FailedID)
	require.NoError(t, service.handover(t.Context()))
	require.NoFileExists(t, filepath.Join(root, "request.json"))
}

func TestBackgroundChecksDoNotRearmDeferredRelease(t *testing.T) {
	client, envelope, manifest, _, _ := fixture(t, "")
	client.HTTP.Transport = transport(func(r *http.Request) (*http.Response, error) {
		var data []byte
		switch {
		case r.URL.Host == "api.github.com":
			data, _ = json.Marshal(map[string]any{"tag_name": manifest.Version, "draft": false, "prerelease": false})
		case strings.HasSuffix(r.URL.Path, "ufc-update.json"):
			data = envelope.Manifest
		case strings.HasSuffix(r.URL.Path, "ufc-update.sig"):
			data = envelope.Signature
		default:
			t.Fatalf("deferred update attempted another download: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}, nil
	})
	repo := &memoryRepo{state: State{Revision: 5, AutoInstall: true, Phase: "deferred", ID: ID(envelope), Version: manifest.Version, Envelope: envelope, Highwater: manifest.Sequence}}
	service := New(repo, "1.0.0", "sqlite", "unused.db", "profile", nil)
	service.client = client
	service.root = t.TempDir()
	restarted := false
	service.shutdown = func() { restarted = true }
	_, err := service.check(t.Context())
	require.NoError(t, err)
	require.Equal(t, "deferred", repo.state.Phase)
	require.Equal(t, int64(5), repo.state.Revision)
	require.NoError(t, service.handover(t.Context()))
	require.False(t, restarted)
}

func TestOwnedCleanupCannotDeleteOutsideStoreOrUnknownFiles(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "media.mkv"), []byte("retained"), 0600))
	require.Error(t, removeOwnedFlat(root, outside, map[string]bool{"media.mkv": true}))
	require.FileExists(t, filepath.Join(outside, "media.mkv"))
	owned := filepath.Join(root, "stage-fixture")
	require.NoError(t, os.Mkdir(owned, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(owned, "unknown.mkv"), []byte("retained"), 0600))
	require.Error(t, removeOwnedFlat(root, owned, map[string]bool{"payload.zip": true}))
	require.FileExists(t, filepath.Join(owned, "unknown.mkv"))
	require.Error(t, Disjoint(root, root))
	require.Error(t, Disjoint(root, filepath.Join(root, "library")))
	require.Error(t, Disjoint(filepath.Join(root, "cache"), root))
}

func TestDrainTimeoutDefersWithoutCancellingCurrentWork(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("in-app handover is supported only on Windows amd64")
	}
	client, envelope, manifest, _, _ := fixture(t, "")
	repo := &memoryRepo{state: State{Revision: 2, AutoInstall: true, Phase: "waiting_for_idle", ID: ID(envelope), Version: manifest.Version, Envelope: envelope}}
	service := New(repo, "1.0.0", "sqlite", "unused.db", "profile", nil)
	service.client = client
	service.root = t.TempDir()
	restarted := false
	service.shutdown = func() { restarted = true }
	service.drainStarted = time.Now().Add(-31 * time.Minute)
	release, ok := maintenance.Application.Enter()
	require.True(t, ok)
	defer release()
	t.Cleanup(maintenance.Application.Resume)
	require.NoError(t, service.handover(t.Context()))
	require.False(t, restarted)
	require.Equal(t, "deferred", repo.state.Phase)
	require.True(t, repo.state.AutoInstall)
	next, ok := maintenance.Application.Enter()
	require.True(t, ok, "normal work must resume after deferral")
	next()
}
