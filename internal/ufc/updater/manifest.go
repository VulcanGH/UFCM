// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

// Package updater verifies and stages UFC Manager releases independently of the upstream updater.
package updater

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Repository is the UFC-owned public release channel.
const Repository = "VulcanGH/UFCM"

// Protocol is the supported launcher activation contract.
const Protocol = 1

// ManifestLimit bounds authenticated metadata before decoding.
const ManifestLimit = 128 << 10

// MaxPayload bounds download and extraction resource use.
const MaxPayload = 256 << 20

// PublicKey is injected at release build time; an unconfigured build cannot install updates.
var PublicKey string

// SchemaDigest binds a release build to its exact upstream and UFC migration sources.
var SchemaDigest string

var versionPattern = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)

// File records an exact permitted runtime file.
type File struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Manifest is signed as exact UTF-8 bytes, never as a reserialized object.
type Manifest struct {
	Format      int       `json:"format"`
	Product     string    `json:"product"`
	Channel     string    `json:"channel"`
	Version     string    `json:"version"`
	Sequence    int64     `json:"sequence"`
	Commit      string    `json:"commit"`
	Schema      string    `json:"schema"`
	Issued      time.Time `json:"issued"`
	Expires     time.Time `json:"expires"`
	OS          string    `json:"os"`
	Arch        string    `json:"arch"`
	MinLauncher int       `json:"min_launcher"`
	URL         string    `json:"url"`
	Size        int64     `json:"size"`
	SHA256      string    `json:"sha256"`
	Files       []File    `json:"files"`
}

// Envelope preserves the bytes needed to reverify staged and installed releases.
type Envelope struct {
	Manifest  []byte `json:"manifest"`
	Signature []byte `json:"signature"`
}

// Trust holds the pinned channel and public key, separate from untrusted release metadata.
type Trust struct {
	Key        ed25519.PublicKey
	Repository string
}

// ProductionTrust never substitutes a test key or upstream channel.
func ProductionTrust() (Trust, error) {
	key, err := base64.StdEncoding.DecodeString(PublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return Trust{}, fmt.Errorf("update signing key is not configured")
	}
	return Trust{Key: key, Repository: Repository}, nil
}

// Verify authenticates metadata and checks bounded, platform-specific release policy.
func (t Trust) Verify(e Envelope, now time.Time, highwater int64, freshness bool) (Manifest, error) {
	var m Manifest
	if len(t.Key) != ed25519.PublicKeySize || len(e.Manifest) == 0 || len(e.Manifest) > ManifestLimit || !ed25519.Verify(t.Key, e.Manifest, e.Signature) {
		return m, fmt.Errorf("invalid update signature")
	}
	d := json.NewDecoder(bytes.NewReader(e.Manifest))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return m, fmt.Errorf("invalid update manifest: %w", err)
	}
	if d.Decode(new(any)) != io.EOF {
		return m, fmt.Errorf("trailing update metadata")
	}
	if m.Format != 1 || m.Product != "ufc-manager" || m.Channel != "stable" || m.OS != "windows" || m.Arch != "amd64" || m.MinLauncher < 1 || m.MinLauncher > Protocol || !versionPattern.MatchString(m.Version) || m.Sequence < 1 || m.Sequence < highwater || len(m.Commit) != 40 {
		return m, fmt.Errorf("incompatible or replayed update")
	}
	if _, err := hex.DecodeString(m.Commit); err != nil {
		return m, fmt.Errorf("invalid source commit")
	}
	if !m.Expires.After(m.Issued) || (freshness && (m.Issued.After(now.Add(10*time.Minute)) || !m.Expires.After(now))) {
		return m, fmt.Errorf("update metadata is expired or not yet valid")
	}
	if !digest(m.Schema) {
		return m, fmt.Errorf("missing migration source digest")
	}
	if !assetURL(m.URL, t.Repository, m.Version) || m.Size < 1 || m.Size > MaxPayload || !digest(m.SHA256) || len(m.Files) < 2 || len(m.Files) > 8 {
		return m, fmt.Errorf("invalid update payload")
	}
	seen := map[string]bool{}
	var total int64
	for _, f := range m.Files {
		if !allowedFile(f.Name) || seen[strings.ToLower(f.Name)] || f.Size < 1 || f.Size > MaxPayload || !digest(f.SHA256) {
			return m, fmt.Errorf("invalid runtime file list")
		}
		seen[strings.ToLower(f.Name)] = true
		total += f.Size
	}
	if total > 2*MaxPayload || !seen["ufc-manager-desktop.exe"] || !seen["ufc-manager.exe"] {
		return m, fmt.Errorf("incomplete runtime")
	}
	return m, nil
}

func allowedFile(name string) bool {
	switch name {
	case "ufc-manager-desktop.exe", "ufc-manager.exe", "LICENSE.txt", "README.txt":
		return true
	}
	return false
}
func digest(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == sha256.Size && s == strings.ToLower(s)
}
func assetURL(raw, repo, version string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "github.com" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && strings.HasPrefix(u.Path, "/"+repo+"/releases/download/"+version+"/") && !strings.Contains(u.Path, "..") && strings.Count(strings.TrimPrefix(u.Path, "/"+repo+"/releases/download/"+version+"/"), "/") == 0
}

// ID binds a release to its authenticated metadata, including its sequence.
func ID(e Envelope) string { sum := sha256.Sum256(e.Manifest); return hex.EncodeToString(sum[:]) }
