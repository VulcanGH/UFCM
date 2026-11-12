// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package library

import (
	"context"
	"crypto/rand"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/autobrr/autobrr/internal/maintenance"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/matching"
	"github.com/autobrr/autobrr/pkg/errors"
)

type repository interface {
	ClearLibrary(context.Context, int64) error
	LibrarySettings(context.Context) (*Settings, error)
	SaveLibraryRoots(context.Context, int64, []Root) (*Settings, error)
	BeginLibraryScan(context.Context, string) error
	FinishLibraryScan(context.Context, string, string, bool) error
	CommitLibraryRoot(context.Context, string, Root, []File, bool) error
	LibraryView(context.Context) (*View, error)
	LibraryFiles(context.Context, string, int) (*FilePage, error)
	LibraryFile(context.Context, string) (File, error)
	EditLibraryFile(context.Context, string, FileEdit, string) (File, error)
	CatalogueMatchingEvents(context.Context) ([]catalogue.Event, error)
	LoadCatalogueEvent(context.Context, string) (catalogue.Event, error)
	AdoptLibraryEvents(context.Context, *catalogue.Snapshot) error
}

// Service owns explicit read-only scans in the existing application lifecycle.
type Service struct {
	repo    repository
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	running bool
}

// New constructs a library service without accessing roots or remote metadata.
func New(r repository) *Service { return &Service{repo: r} }

// Start records interrupted work without automatically scanning configured media.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	settings, err := s.repo.LibrarySettings(ctx)
	if err != nil {
		return err
	}
	if settings.Running {
		if err := s.repo.FinishLibraryScan(ctx, settings.RunID, "Previous scan interrupted; saved inventory retained", true); err != nil {
			return err
		}
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.done = make(chan struct{})
	close(s.done)
	return nil
}

// Stop cancels active scanning and joins it before database closure.
func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	done := s.done
	s.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Settings reads configured and retained locations without scanning them.
func (s *Service) Settings(ctx context.Context) (*Settings, error) {
	return s.repo.LibrarySettings(ctx)
}

// Clear removes saved observations while preserving roots and the event catalogue.
func (s *Service) Clear(ctx context.Context, revision int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running || revision < 1 {
		return catalogue.ErrRevisionConflict
	}
	return s.repo.ClearLibrary(ctx, revision)
}

func normalized(value string) string {
	value = filepath.Clean(value)
	if runtime.GOOS == "windows" {
		value = strings.ToLower(value)
	}
	return value
}

// SaveSettings validates absolute non-overlapping roots and preserves inventory.
func (s *Service) SaveSettings(ctx context.Context, input SettingsEdit) (*Settings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if input.Revision < 1 || len(input.Paths) > 8 {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "choose up to eight absolute library roots")
	}
	roots := []Root{}
	for _, value := range input.Paths {
		value = filepath.Clean(strings.TrimSpace(value))
		if !filepath.IsAbs(value) || len(value) > 4096 {
			return nil, errors.Wrap(catalogue.ErrInvalidEdit, "library roots must be absolute paths")
		}
		for _, prior := range roots {
			for _, pair := range [][2]string{{prior.Path, value}, {value, prior.Path}} {
				rel, err := filepath.Rel(normalized(pair[0]), normalized(pair[1]))
				if err == nil && (rel == "." || rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))) {
					return nil, errors.Wrap(catalogue.ErrInvalidEdit, "library roots must not overlap")
				}
			}
		}
		roots = append(roots, Root{ID: Key(normalized(value)), Path: value, Enabled: true, State: "unscanned"})
	}
	return s.repo.SaveLibraryRoots(ctx, input.Revision, roots)
}

// View reads compact event coverage from saved inventory only.
func (s *Service) View(ctx context.Context) (*View, error) { return s.repo.LibraryView(ctx) }

// Files reads a bounded expanded group without opening media files.
func (s *Service) Files(ctx context.Context, group string, offset int) (*FilePage, error) {
	if len(group) > 256 || offset < 0 || offset > 100000 {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "invalid library page")
	}
	return s.repo.LibraryFiles(ctx, group, offset)
}

// Events lists retained canonical identities for protected manual assignment.
func (s *Service) Events(ctx context.Context) ([]EventOption, error) {
	events, err := s.repo.CatalogueMatchingEvents(ctx)
	if err != nil {
		return nil, err
	}
	result := []EventOption{}
	for _, e := range events {
		result = append(result, EventOption{ID: e.ID, Name: e.Values.Name, Date: e.Values.Date})
	}
	return result, nil
}

// Edit saves reviewed identification or explicitly restores detected values.
func (s *Service) Edit(ctx context.Context, id string, input FileEdit, actor string) (File, error) {
	if input.Revision < 1 {
		return File{}, catalogue.ErrRevisionConflict
	}
	if input.Mode == "confirm" {
		if err := catalogue.ValidateOverrides(map[string]bool{input.Component: true}); err != nil {
			return File{}, errors.Wrap(catalogue.ErrInvalidEdit, "choose a supported component")
		}
		if input.Resolution != "unknown" {
			if err := catalogue.ValidateResolutions([]string{input.Resolution}); err != nil {
				return File{}, err
			}
		}
		if _, err := s.repo.LoadCatalogueEvent(ctx, input.EventID); err != nil {
			return File{}, err
		}
	}
	return s.repo.EditLibraryFile(ctx, id, input, actor)
}

// Scan acknowledges an explicit bounded scan after persisting its intent.
func (s *Service) Scan(ctx context.Context) (*Accepted, error) {
	ctx, release, allowed := maintenance.Application.EnterContext(ctx)
	if !allowed {
		return nil, errors.New("application is preparing an update")
	}
	transferred := false
	defer func() {
		if !transferred {
			release()
		}
	}()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx == nil || s.ctx.Err() != nil {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "library service unavailable")
	}
	if s.running {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "library scan already running")
	}
	settings, err := s.repo.LibrarySettings(ctx)
	if err != nil {
		return nil, err
	}
	roots := []Root{}
	for _, root := range settings.Roots {
		if root.Enabled {
			roots = append(roots, root)
		}
	}
	if len(roots) == 0 {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "configure library roots first")
	}
	id := rand.Text()
	if err := s.repo.BeginLibraryScan(ctx, id); err != nil {
		return nil, err
	}
	s.running = true
	s.done = make(chan struct{})
	transferred = true
	go func() { defer release(); s.scan(id, roots) }()
	return &Accepted{RunID: id}, nil
}

func walkRoot(ctx context.Context, root Root, limit int) ([]File, bool) {
	files := []File{}
	info, err := os.Lstat(root.Path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || reparse(info) {
		return files, false
	}
	handle, err := os.OpenRoot(root.Path)
	if err != nil {
		return files, false
	}
	defer handle.Close()
	return walkFiles(ctx, root, handle.FS(), limit)
}

func walkFiles(ctx context.Context, root Root, filesystem fs.FS, limit int) ([]File, bool) {
	files := []File{}
	complete := true
	err := fs.WalkDir(filesystem, ".", func(name string, entry fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			complete = false
			return nil
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() && name != "." && (strings.HasPrefix(entry.Name(), ".") || strings.Count(name, "/") > 64) {
			return fs.SkipDir
		}
		info, err := entry.Info()
		if err != nil {
			complete = false
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if reparse(info) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		ext := strings.ToLower(path.Ext(name))
		switch ext {
		case ".mkv", ".mp4", ".avi", ".m4v", ".ts", ".m2ts", ".mov", ".webm":
		default:
			return nil
		}
		if len(files) >= limit {
			return errors.New("library file limit reached")
		}
		files = append(files, File{ID: Key(root.ID + "\x00" + normalized(filepath.FromSlash(name))), RootID: root.ID, RootPath: root.Path, Path: name, Size: info.Size(), Modified: info.ModTime().UTC().Format(time.RFC3339Nano), Present: true})
		return nil
	})
	return files, complete && err == nil
}

func (s *Service) scan(run string, roots []Root) {
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Minute)
	defer cancel()
	message := "Scan finished"
	interrupted := true
	defer func() {
		saveCtx, stop := context.WithTimeout(context.WithoutCancel(s.ctx), 5*time.Second)
		defer stop()
		if err := s.repo.FinishLibraryScan(saveCtx, run, message, interrupted); err != nil {
			message = "Scan status could not be saved"
		}
		s.mu.Lock()
		s.running = false
		close(s.done)
		s.mu.Unlock()
	}()
	events, err := s.repo.CatalogueMatchingEvents(ctx)
	if err != nil {
		message = "Catalogue unavailable; saved inventory retained"
		return
	}
	type observation struct {
		root     Root
		files    []File
		complete bool
	}
	observations := []observation{}
	remaining := 100000
	for _, root := range roots {
		files, complete := walkRoot(ctx, root, remaining)
		remaining -= len(files)
		observations = append(observations, observation{root, files, complete})
		if !complete {
			message = "Scan finished with unavailable or incomplete locations; saved inventory retained"
		}
	}
	if ctx.Err() != nil {
		message = "Scan interrupted; saved inventory retained"
		return
	}
	cache := map[string]matching.Preview{}
	for _, o := range observations {
		files := make([]File, 0, len(o.files))
		for _, f := range o.files {
			if ctx.Err() != nil {
				message = "Scan interrupted; saved inventory retained"
				return
			}
			files = append(files, Identify(f, events, cache))
		}
		if err := s.repo.CommitLibraryRoot(ctx, run, o.root, files, o.complete); err != nil {
			message = "Inventory could not be saved; previous observations retained"
			return
		}
	}
	interrupted = false
}
