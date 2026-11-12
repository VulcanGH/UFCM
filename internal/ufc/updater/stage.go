// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package updater

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Client fetches bounded public release assets; it never carries GitHub credentials.
type Client struct {
	HTTP  *http.Client
	Trust Trust
}

var errNoRelease = errors.New("no public stable release")

// NewClient creates a bounded downloader with HTTPS-only redirects.
func NewClient(t Trust) *Client {
	return &Client{Trust: t, HTTP: &http.Client{Timeout: 10 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 || r.URL.Scheme != "https" || r.URL.User != nil {
			return fmt.Errorf("unsafe update redirect")
		}
		return nil
	}}}
}

func (c *Client) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "UFC-Manager-Updater/1")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		if res.StatusCode == http.StatusNotFound && url == "https://api.github.com/repos/"+c.Trust.Repository+"/releases/latest" {
			return nil, errNoRelease
		}
		return nil, fmt.Errorf("release server returned HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("update response exceeds limit")
	}
	return data, nil
}

// Latest ignores drafts and prereleases and authenticates the release before download.
func (c *Client) Latest(ctx context.Context, highwater int64) (Envelope, Manifest, error) {
	var release struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	data, err := c.get(ctx, "https://api.github.com/repos/"+c.Trust.Repository+"/releases/latest", 1<<20)
	if err != nil {
		return Envelope{}, Manifest{}, err
	}
	if err = json.Unmarshal(data, &release); err != nil || release.Draft || release.Prerelease || !versionPattern.MatchString(release.Tag) {
		return Envelope{}, Manifest{}, fmt.Errorf("no compatible stable release")
	}
	base := "https://github.com/" + c.Trust.Repository + "/releases/download/" + release.Tag + "/"
	raw, err := c.get(ctx, base+"ufc-update.json", ManifestLimit)
	if err != nil {
		return Envelope{}, Manifest{}, err
	}
	sig, err := c.get(ctx, base+"ufc-update.sig", 128)
	if err != nil {
		return Envelope{}, Manifest{}, err
	}
	e := Envelope{Manifest: raw, Signature: sig}
	m, err := c.Trust.Verify(e, time.Now(), highwater, true)
	if err == nil && m.Version != release.Tag {
		err = fmt.Errorf("release tag does not match signed version")
	}
	return e, m, err
}

// SafeDirectory rejects symlinks and Windows reparse points in every existing ancestor.
func SafeDirectory(path string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for {
		info, err := os.Lstat(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0 || reparse(info)) {
			return fmt.Errorf("unsafe update directory")
		}
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	return nil
}

// Stage downloads into a new owned directory and publishes only a fully verified runtime.
func (c *Client) Stage(ctx context.Context, root string, e Envelope, highwater int64) (string, error) {
	m, err := c.Trust.Verify(e, time.Now(), highwater, true)
	if err != nil {
		return "", err
	}
	if err = SafeDirectory(root); err != nil {
		return "", err
	}
	if err = os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	final := filepath.Join(root, ID(e))
	if _, err = os.Lstat(final); err == nil {
		return final, c.VerifyRuntime(final, e, highwater)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	if len(entries) >= 8 {
		return "", fmt.Errorf("runtime retention limit reached; existing versions were retained for recovery")
	}
	temp, err := os.MkdirTemp(root, "stage-")
	if err != nil {
		return "", err
	}
	published := false
	defer func() {
		if !published {
			allowed := map[string]bool{"payload.zip": true, "release.json": true}
			for _, file := range m.Files {
				allowed[file.Name] = true
			}
			_ = removeOwnedFlat(root, temp, allowed)
		}
	}()
	data, err := c.get(ctx, m.URL, m.Size)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	if int64(len(data)) != m.Size || hex.EncodeToString(sum[:]) != m.SHA256 {
		return "", fmt.Errorf("payload size or hash mismatch")
	}
	archive := filepath.Join(temp, "payload.zip")
	if err = os.WriteFile(archive, data, 0600); err != nil {
		return "", err
	}
	z, err := zip.OpenReader(archive)
	if err != nil {
		return "", err
	}
	expected := map[string]File{}
	for _, f := range m.Files {
		expected[f.Name] = f
	}
	seen := map[string]bool{}
	for _, f := range z.File {
		spec, ok := expected[f.Name]
		if !ok || seen[strings.ToLower(f.Name)] || !f.Mode().IsRegular() || f.UncompressedSize64 != uint64(spec.Size) {
			z.Close()
			return "", fmt.Errorf("unsafe or unexpected archive entry")
		}
		seen[strings.ToLower(f.Name)] = true
		input, err := f.Open()
		if err != nil {
			z.Close()
			return "", err
		}
		output, err := os.OpenFile(filepath.Join(temp, f.Name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
		if err != nil {
			input.Close()
			z.Close()
			return "", err
		}
		h := sha256.New()
		n, copyErr := io.Copy(io.MultiWriter(output, h), io.LimitReader(input, spec.Size+1))
		syncErr := output.Sync()
		closeErr := output.Close()
		input.Close()
		if copyErr != nil || syncErr != nil || closeErr != nil || n != spec.Size || hex.EncodeToString(h.Sum(nil)) != spec.SHA256 {
			z.Close()
			return "", fmt.Errorf("runtime extraction verification failed")
		}
	}
	if err = z.Close(); err != nil {
		return "", err
	}
	if len(seen) != len(expected) {
		return "", fmt.Errorf("missing runtime files")
	}
	if err = os.Remove(archive); err != nil {
		return "", err
	}
	if err = WriteJSON(filepath.Join(temp, "release.json"), e); err != nil {
		return "", err
	}
	if err = os.Rename(temp, final); err != nil {
		return "", err
	}
	published = true
	return final, nil
}

// VerifyRuntime checks every executable again before it can be launched.
func (c *Client) VerifyRuntime(dir string, e Envelope, highwater int64) error {
	m, err := c.Trust.Verify(e, time.Now(), highwater, false)
	if err != nil {
		return err
	}
	if err = SafeDirectory(dir); err != nil {
		return err
	}
	for _, f := range m.Files {
		path := filepath.Join(dir, f.Name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || reparse(info) || info.Size() != f.Size {
			return fmt.Errorf("runtime file unavailable or unsafe")
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		h := sha256.New()
		_, err = io.Copy(h, in)
		in.Close()
		if err != nil || hex.EncodeToString(h.Sum(nil)) != f.SHA256 {
			return fmt.Errorf("runtime file hash mismatch")
		}
	}
	return nil
}

// WriteJSON atomically replaces an owned journal after flushing its contents.
func WriteJSON(path string, value any) error {
	if err := SafeDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), "journal-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return replaceFile(name, path)
}
