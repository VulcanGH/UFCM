// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package updater

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/autobrr/autobrr/internal/maintenance"

	"github.com/stretchr/testify/require"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func hash(data []byte) string                                         { s := sha256.Sum256(data); return hex.EncodeToString(s[:]) }

func fixture(t *testing.T, extra string) (*Client, Envelope, Manifest, []byte, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	var data bytes.Buffer
	z := zip.NewWriter(&data)
	files := []File{}
	for _, name := range []string{"ufc-manager-desktop.exe", "ufc-manager.exe"} {
		payload := []byte("fixture-" + name)
		w, err := z.Create(name)
		require.NoError(t, err)
		_, err = w.Write(payload)
		require.NoError(t, err)
		files = append(files, File{Name: name, Size: int64(len(payload)), SHA256: hash(payload)})
	}
	if extra != "" {
		w, err := z.Create(extra)
		require.NoError(t, err)
		_, err = w.Write([]byte("unexpected"))
		require.NoError(t, err)
	}
	require.NoError(t, z.Close())
	payload := data.Bytes()
	now := time.Now().UTC()
	m := Manifest{Format: 1, Product: "ufc-manager", Channel: "stable", Version: "v1.0.1", Sequence: 2, Commit: fmt.Sprintf("%040d", 1), Issued: now.Add(-time.Hour), Expires: now.Add(time.Hour), OS: "windows", Arch: "amd64", MinLauncher: 1, URL: "https://github.com/" + Repository + "/releases/download/v1.0.1/runtime.zip", Size: int64(len(payload)), SHA256: hash(payload), Files: files}
	m.Schema = hash([]byte("fixture-schema"))
	raw, err := json.Marshal(m)
	require.NoError(t, err)
	e := Envelope{Manifest: raw, Signature: ed25519.Sign(private, raw)}
	c := NewClient(Trust{Key: public, Repository: Repository})
	c.HTTP = &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(payload)), Header: make(http.Header)}, nil
	})}
	return c, e, m, payload, private
}

func TestManifestRejectsUntrustedExpiredReplayAndWrongChannel(t *testing.T) {
	c, e, m, _, key := fixture(t, "")
	_, err := c.Trust.Verify(e, time.Now(), 2, true)
	require.NoError(t, err)
	altered := e
	altered.Manifest = append(append([]byte{}, e.Manifest...), byte(' '))
	_, err = c.Trust.Verify(altered, time.Now(), 0, true)
	require.Error(t, err)
	for _, change := range []func(*Manifest){func(m *Manifest) { m.Product = "autobrr" }, func(m *Manifest) { m.Channel = "beta" }, func(m *Manifest) { m.Arch = "arm64" }, func(m *Manifest) { m.Expires = time.Now().Add(-time.Minute) }, func(m *Manifest) { m.URL = "https://github.com/autobrr/autobrr/releases/download/v1.0.1/file.zip" }, func(m *Manifest) { m.MinLauncher = 2 }, func(m *Manifest) { m.Files[0].Name = "../outside.exe" }} {
		candidate := m
		candidate.Files = append([]File{}, m.Files...)
		change(&candidate)
		raw, _ := json.Marshal(candidate)
		_, err := c.Trust.Verify(Envelope{Manifest: raw, Signature: ed25519.Sign(key, raw)}, time.Now(), 0, true)
		require.Error(t, err)
	}
	_, err = c.Trust.Verify(e, time.Now(), 3, true)
	require.Error(t, err)
	_, err = c.Trust.Verify(e, time.Now().Add(-24*time.Hour), 0, false)
	require.NoError(t, err, "clock skew must not prevent an installed runtime starting")
	_, err = (Trust{Repository: Repository}).Verify(e, time.Now(), 0, true)
	require.Error(t, err)
}

func TestStageRejectsArchiveEscapesTamperingAndInterruptedTransfers(t *testing.T) {
	for _, entry := range []string{"../outside.exe", "C:/outside.exe", "nested/file", "UFC-MANAGER.EXE"} {
		t.Run(entry, func(t *testing.T) {
			c, e, _, _, _ := fixture(t, entry)
			root := t.TempDir()
			_, err := c.Stage(t.Context(), root, e, 0)
			require.Error(t, err)
			require.NoFileExists(t, filepath.Join(filepath.Dir(root), "outside.exe"))
		})
	}
	c, e, _, _, _ := fixture(t, "")
	root := t.TempDir()
	dir, err := c.Stage(t.Context(), root, e, 0)
	require.NoError(t, err)
	require.NoError(t, c.VerifyRuntime(dir, e, 2))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "ufc-manager.exe"), []byte("tampered"), 0600))
	require.Error(t, c.VerifyRuntime(dir, e, 2))
	c.HTTP.Transport = transport(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader([]byte("truncated"))), Header: make(http.Header)}, nil
	})
	other := t.TempDir()
	_, err = c.Stage(t.Context(), other, e, 0)
	require.Error(t, err)
	require.NoDirExists(t, filepath.Join(other, ID(e)))
}

type memoryRepo struct {
	mu    sync.Mutex
	state State
}

func (r *memoryRepo) UpdateState(context.Context) (State, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state, nil
}
func (r *memoryRepo) SaveUpdateState(_ context.Context, s State) (State, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s.Revision != r.state.Revision || s.Highwater < r.state.Highwater {
		return State{}, fmt.Errorf("revision conflict")
	}
	s.Revision++
	r.state = s
	return s, nil
}

func TestPostgresRejectsManualAndAutomaticInstallation(t *testing.T) {
	repo := &memoryRepo{state: State{Revision: 1, Phase: "verified", ID: "release"}}
	s := New(repo, "1.0.0", "postgres", "unused", "unused", nil)
	status, err := s.Status(t.Context())
	require.NoError(t, err)
	require.False(t, status.Supported)
	require.Contains(t, status.Reason, "PostgreSQL")
	_, err = s.Install(t.Context(), "release")
	require.ErrorContains(t, err, "PostgreSQL")
	_, err = s.Preferences(t.Context(), 1, true)
	require.ErrorContains(t, err, "PostgreSQL")
	require.NoError(t, s.handover(t.Context()))
	require.Equal(t, int64(1), repo.state.Revision)
}

func TestInstallWaitsForWorkAndPreservesIdempotency(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("in-app installation is supported only on Windows amd64")
	}
	c, e, _, _, _ := fixture(t, "")
	root := t.TempDir()
	_, err := c.Stage(t.Context(), filepath.Join(root, "versions"), e, 0)
	require.NoError(t, err)
	repo := &memoryRepo{state: State{Revision: 1, Phase: "verified", ID: ID(e), Version: "v1.0.1", Envelope: e, Highwater: 2}}
	s := New(repo, "1.0.0", "sqlite", "database.db", "profile", nil)
	s.client = c
	s.root = root
	stopped := false
	s.shutdown = func() { stopped = true }
	release, ok := maintenance.Application.Enter()
	require.True(t, ok)
	_, err = s.Install(t.Context(), ID(e))
	require.NoError(t, err)
	revision := repo.state.Revision
	_, err = s.Install(t.Context(), ID(e))
	require.NoError(t, err)
	require.Equal(t, revision, repo.state.Revision)
	require.NoError(t, s.handover(t.Context()))
	require.False(t, stopped)
	release()
	require.NoError(t, s.handover(t.Context()))
	t.Cleanup(maintenance.Application.Resume)
	require.True(t, stopped)
	var request Request
	require.NoError(t, ReadJSON(filepath.Join(root, "request.json"), &request))
	require.Equal(t, ID(e), ID(request.Envelope))
	_, ok = maintenance.Application.Enter()
	require.False(t, ok)
}
