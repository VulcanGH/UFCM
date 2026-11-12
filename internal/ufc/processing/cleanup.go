// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package processing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

func (s *Service) cleanupCompleted(ctx context.Context, job Job) string {
	if job.State != "completed" || !job.CleanupRequested || len(job.Files) == 0 || len(job.Files) > 8 {
		return "Copy evidence is incomplete; source retained"
	}
	settings, err := s.repo.ProcessingSettings(ctx)
	if err != nil || settings == nil || !settings.CleanupSABDownload {
		return "Cleanup is no longer enabled; source retained"
	}
	acquisition, err := s.repo.Acquisition(ctx, job.AcquisitionID)
	if err != nil || acquisition.ID != job.AcquisitionID || acquisition.State != "completed" || acquisition.RemoteID == "" || acquisition.Storage == "" || acquisition.EventID != job.EventID || acquisition.ClientID != settings.SABClientID || acquisition.Category != settings.SABCategory {
		return "Completed UFC download identity changed; source retained"
	}
	storage := filepath.Clean(acquisition.Storage)
	if !filepath.IsAbs(storage) || storage == filepath.VolumeName(storage)+string(filepath.Separator) {
		return "Completed storage path is unsafe; source retained"
	}
	owned := cleanupMarker(filepath.Base(storage), acquisition.ID)
	for _, file := range job.Files {
		owned = owned || cleanupMarker(filepath.Base(filepath.FromSlash(file.Source)), acquisition.ID)
	}
	if !owned && s.downloaders != nil {
		client, clientErr := s.sabClient(ctx, acquisition.ClientID)
		if clientErr == nil {
			checkCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			remote, lookupErr := client.Lookup(checkCtx, acquisition.RemoteID, "")
			cancel()
			owned = lookupErr == nil && remote.Found && !remote.Ambiguous && remote.History && remote.ID == acquisition.RemoteID && strings.EqualFold(remote.State, "Completed") && sameCleanupPath(remote.Storage, storage) && cleanupMarker(remote.Title, acquisition.ID)
		}
	}
	if !owned {
		return "UFC ownership could not be confirmed in the folder, filename or exact SAB history job; source retained"
	}
	selectedRoot := ""
	for _, root := range settings.SourceRoots {
		if sameCleanupPath(root, storage) {
			return "Completed storage is a configured source folder; source retained"
		}
		if filepath.IsAbs(root) && within(root, storage) && len(root) > len(selectedRoot) {
			selectedRoot = root
		}
	}
	if selectedRoot == "" {
		return "Completed storage is outside configured source folders"
	}
	resolvedRoot, err := filepath.EvalSymlinks(selectedRoot)
	if err != nil {
		return "Completed source folder is unavailable"
	}
	resolved, err := filepath.EvalSymlinks(storage)
	missing := os.IsNotExist(err)
	if missing {
		parent := filepath.Dir(storage)
		var resolvedParent string
		var parentErr error
		for within(selectedRoot, parent) {
			resolvedParent, parentErr = filepath.EvalSymlinks(parent)
			if parentErr == nil || !os.IsNotExist(parentErr) || sameCleanupPath(parent, selectedRoot) {
				break
			}
			parent = filepath.Dir(parent)
		}
		if parentErr != nil {
			return "Completed storage parent is unavailable; source retained"
		}
		relative, relativeErr := filepath.Rel(parent, storage)
		if relativeErr != nil || resolvedParent == "" {
			return "Completed storage path is unsafe; source retained"
		}
		resolved = filepath.Join(resolvedParent, relative)
	}
	if err != nil && !missing || !within(resolvedRoot, resolved) || sameCleanupPath(resolved, resolvedRoot) {
		return "Completed storage escaped its configured source folder"
	}
	view, err := s.repo.LibraryView(ctx)
	if err != nil || view == nil || view.Settings == nil {
		return "Library protection could not be checked"
	}
	for _, root := range view.Settings.Roots {
		if overlaps(storage, root.Path) || overlaps(resolved, root.Path) {
			return "Completed storage overlaps a protected library folder"
		}
		if linked, err := filepath.EvalSymlinks(root.Path); err == nil && (overlaps(storage, linked) || overlaps(resolved, linked)) {
			return "Completed storage overlaps a protected library folder"
		}
	}
	info, err := os.Lstat(storage)
	if !missing && (err != nil || cleanupLinked(storage, info) || (!info.IsDir() && !info.Mode().IsRegular())) {
		return "Completed storage is unavailable or linked"
	}
	if missing && !os.IsNotExist(err) {
		return "Completed storage state changed; inspect completed download"
	}
	expected := make(map[string]JobFile, len(job.Files))
	for _, file := range job.Files {
		if file.State != "verified" || file.SourceSHA256 == "" || file.SourceSHA256 != file.DestinationSHA256 || file.Size <= 0 {
			return "Not every intended file has a durable verified copy"
		}
		source := storage
		singleFile := !missing && info.Mode().IsRegular() || missing && len(job.Files) == 1 && filepath.Base(storage) == file.Source
		if !singleFile {
			source = filepath.Join(storage, filepath.FromSlash(file.Source))
		}
		if !within(storage, source) || sameCleanupPath(source, storage) && !singleFile {
			return "A recorded source path is unsafe"
		}
		expected[cleanupPathKey(source)] = file
		destination := filepath.Join(job.DestinationRoot, filepath.FromSlash(file.Destination))
		if !within(job.DestinationRoot, destination) || destination == job.DestinationRoot {
			return "A recorded Library path is unsafe"
		}
		destInfo, err := os.Lstat(destination)
		if err != nil || !destInfo.Mode().IsRegular() || destInfo.Size() != file.Size {
			return "Verified Library copy changed; source retained"
		}
		if !safeExistingFile(job.DestinationRoot, file.Destination) {
			return "Verified Library copy is linked or unavailable; source retained"
		}
		hash, _, hashErr := hashCleanupFile(ctx, destination)
		if hashErr != nil || hash != file.DestinationSHA256 {
			return "Library copy could not be reverified; source retained"
		}
	}
	if missing {
		return ""
	}
	files, dirs := []string{}, []string{}
	observed := map[string]os.FileInfo{storage: info}
	if info.IsDir() {
		blocked := "Completed storage contains unverified or unsafe entries; source retained"
		err = filepath.WalkDir(storage, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			entryInfo, err := os.Lstat(path)
			if err != nil {
				return err
			}
			if cleanupLinked(path, entryInfo) {
				blocked = "Completed storage contains a linked entry; source retained"
				return os.ErrInvalid
			}
			observed[path] = entryInfo
			if entryInfo.IsDir() {
				if len(dirs) >= 10000 {
					blocked = "Completed storage contains too many directories; source retained"
					return os.ErrInvalid
				}
				dirs = append(dirs, path)
				return nil
			}
			if !entryInfo.Mode().IsRegular() || len(files) >= 10000 {
				blocked = "Completed storage contains an unsupported entry or too many files; source retained"
				return os.ErrInvalid
			}
			files = append(files, path)
			return nil
		})
		if err != nil {
			return blocked
		}
	} else {
		files = append(files, storage)
	}
	// Interrupted cleanup may already have removed sources; every Library copy was reverified above.
	for _, path := range files {
		file, ok := expected[cleanupPathKey(path)]
		if !ok {
			continue
		}
		sourceInfo, err := os.Lstat(path)
		if err != nil || !sourceInfo.Mode().IsRegular() || sourceInfo.Size() != file.Size {
			return "Completed source changed since copy; source retained"
		}
		hash, verifiedInfo, hashErr := hashCleanupFile(ctx, path)
		if hashErr != nil || hash != file.SourceSHA256 {
			return "Completed source changed or is in use; source retained"
		}
		observed[path] = verifiedInfo
	}
	if len(expected) != len(job.Files) {
		return "Copy evidence contains duplicate source paths; source retained"
	}
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return "Cleanup interrupted; inspect completed download"
		}
		if err := removeCleanupEntry(ctx, selectedRoot, path, observed[path]); err != nil {
			return fmt.Sprintf("Could not delete %s: %s. Verified Library copies are retained; retry source cleanup", filepath.Base(path), err)
		}
	}
	sort.Slice(dirs, func(i, j int) bool { return len(dirs[i]) > len(dirs[j]) })
	for _, path := range dirs {
		if err := removeCleanupEntry(ctx, selectedRoot, path, observed[path]); err != nil {
			return fmt.Sprintf("Could not delete folder %s: %s. Retry source cleanup", filepath.Base(path), err)
		}
	}
	return ""
}

func sameCleanupPath(a, b string) bool {
	return a != "" && b != "" && within(a, b) && within(b, a)
}

func cleanupPathKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func cleanupMarker(name, id string) bool {
	if id == "" {
		return false
	}
	name, marker := strings.ToUpper(name), "UFCM-"+strings.ToUpper(id)
	for offset := 0; offset < len(name); {
		index := strings.Index(name[offset:], marker)
		if index < 0 {
			return false
		}
		index += offset
		end := index + len(marker)
		alphanumeric := func(b byte) bool { return b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' }
		if (index == 0 || !alphanumeric(name[index-1])) && (end == len(name) || !alphanumeric(name[end])) {
			return true
		}
		offset = end
	}
	return false
}

func hashCleanupFile(ctx context.Context, path string) (string, os.FileInfo, error) {
	for attempt := 0; ; attempt++ {
		hash, info, err := hashFile(ctx, path)
		if err == nil || attempt == 2 {
			return hash, info, err
		}
		if err := waitCleanupRetry(ctx, attempt+2); err != nil {
			return "", nil, err
		}
	}
}

func waitCleanupRetry(ctx context.Context, seconds int) error {
	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("cleanup interrupted")
	case <-timer.C:
		return nil
	}
}

func removeCleanupEntry(ctx context.Context, root, path string, observed os.FileInfo) error {
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			if err := waitCleanupRetry(ctx, attempt+1); err != nil {
				return err
			}
		}
		if ctx.Err() != nil {
			return fmt.Errorf("cleanup interrupted")
		}
		current, err := os.Lstat(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil || observed == nil || !os.SameFile(current, observed) || !current.IsDir() && (current.Size() != observed.Size() || !current.ModTime().Equal(observed.ModTime())) {
			return fmt.Errorf("source entry changed or is unavailable")
		}
		if !within(root, path) || sameCleanupPath(root, path) {
			return fmt.Errorf("source path is unsafe")
		}
		for parent := path; ; parent = filepath.Dir(parent) {
			info, statErr := os.Lstat(parent)
			if statErr != nil || cleanupLinked(parent, info) {
				return fmt.Errorf("source path is linked or unavailable")
			}
			if sameCleanupPath(parent, root) {
				break
			}
		}
		if err = os.Remove(path); err == nil || os.IsNotExist(err) {
			return nil
		}
		if os.IsPermission(err) && current.Mode().IsRegular() {
			if writableErr := cleanupWritable(path, current); writableErr != nil {
				if attempt == 2 {
					return fmt.Errorf("source is read-only or access is denied")
				}
			} else {
				after, statErr := os.Lstat(path)
				if statErr != nil || cleanupLinked(path, after) || !os.SameFile(current, after) || current.Size() != after.Size() || !current.ModTime().Equal(after.ModTime()) {
					return fmt.Errorf("source entry changed while clearing read-only status")
				}
				if err := os.Remove(path); err == nil || os.IsNotExist(err) {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("source is still in use, access is denied, or the directory is not empty after three attempts")
}
