// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package processing

import (
	"bytes"
	"context"
	"crypto/rand"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/autobrr/autobrr/internal/maintenance"
	"github.com/autobrr/autobrr/internal/ufc/plexplaceholder"
	"github.com/rs/zerolog/log"
)

//go:embed ufcm-main-event-unavailable.mkv
var placeholderVideo []byte

// The hidden hard link proves the visible file was published by UFCM.
const placeholderOwner = ".ufcm-plex-placeholder"

type PlaceholderTask struct {
	JobID, Source, Root, Destination, Component string
	Size                                        int64
}

func (s *Service) reconcilePlaceholders(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		workCtx, release, allowed := maintenance.Application.EnterContext(ctx)
		if allowed {
			s.processPlaceholderTasks(workCtx)
			release()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) processPlaceholderTasks(ctx context.Context) {
	now := time.Now().UTC()
	tasks, err := s.repo.PendingPlaceholders(ctx, now)
	if err != nil || len(tasks) == 0 {
		return
	}
	settings, err := s.repo.ProcessingSettings(ctx)
	if err != nil {
		return
	}
	for _, task := range tasks {
		if ctx.Err() != nil {
			return
		}
		var retry time.Time
		if err := reconcilePlaceholder(task, settings); err != nil {
			log.Warn().Err(err).Str("job_id", task.JobID).Msg("Plex placeholder could not be reconciled")
			retry = now.Add(time.Minute)
		}
		if err := s.repo.FinishPlaceholder(ctx, task, retry); err != nil {
			log.Warn().Err(err).Str("job_id", task.JobID).Msg("Plex placeholder progress could not be saved")
		}
	}
}

func reconcilePlaceholder(task PlaceholderTask, settings *Settings) error {
	main := task.Component == "main_card"
	if !main && !settings.PlexPlaceholder {
		return nil
	}
	final, ok := jobPath(Job{DestinationRoot: task.Root}, task.Destination)
	if !ok {
		return fmt.Errorf("invalid verified media path")
	}
	parts := strings.Split(task.Destination, "/")
	if main && len(parts) != 2 || !main && len(parts) != 3 {
		return fmt.Errorf("invalid event folder layout")
	}
	eventPath := filepath.Join(task.Root, parts[0])
	for _, path := range []string{task.Root, eventPath, filepath.Dir(final), final} {
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if cleanupLinked(path, info) {
			return fmt.Errorf("linked media or event folder")
		}
	}
	event, err := os.OpenRoot(eventPath)
	if err != nil {
		return err
	}
	defer event.Close()
	media, err := event.Lstat(filepath.Join(parts[1:]...))
	if err != nil {
		return err
	}
	if !media.Mode().IsRegular() || media.Size() != task.Size || media.Size() == 0 {
		return fmt.Errorf("verified media changed")
	}
	name := parts[0] + plexplaceholder.Suffix
	if main {
		return removePlaceholder(event, name)
	}

	dir, err := event.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	for {
		entries, err := dir.ReadDir(64)
		for _, entry := range entries {
			ext := strings.ToLower(filepath.Ext(entry.Name()))
			if entry.IsDir() || !slices.ContainsFunc(settings.Extensions, func(configured string) bool { return strings.EqualFold(configured, ext) }) &&
				!slices.Contains([]string{".mkv", ".mp4", ".avi", ".m4v", ".ts", ".m2ts", ".mov", ".webm", ".mpg", ".mpeg", ".wmv", ".asf", ".vob", ".flv", ".3gp"}, ext) {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.Size() > 0 || !info.Mode().IsRegular() {
				return nil
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
	}
	if _, err := event.Lstat(name); !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := event.Lstat(placeholderOwner); errors.Is(err, os.ErrNotExist) {
		if err = stagePlaceholder(event); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if _, err := verifiedPlaceholderOwner(event); err != nil {
		return err
	}
	return event.Link(placeholderOwner, name)
}

func stagePlaceholder(event *os.Root) error {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return err
	}
	stage := fmt.Sprintf(".ufcm-plex-%x.part", token)
	file, err := event.OpenFile(stage, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer event.Remove(stage)
	_, writeErr := file.Write(placeholderVideo)
	if err := errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
		return err
	}
	return event.Link(stage, placeholderOwner)
}

func verifiedPlaceholderOwner(event *os.Root) (os.FileInfo, error) {
	owner, err := event.Lstat(placeholderOwner)
	if err != nil {
		return nil, err
	}
	if !owner.Mode().IsRegular() || owner.Size() != int64(len(placeholderVideo)) {
		return nil, fmt.Errorf("placeholder ownership file changed")
	}
	file, err := event.Open(placeholderOwner)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(owner, opened) {
		return nil, fmt.Errorf("placeholder ownership file changed")
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(len(placeholderVideo))+1))
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(data, placeholderVideo) {
		return nil, fmt.Errorf("placeholder content changed")
	}
	return owner, nil
}

func removePlaceholder(event *os.Root, name string) error {
	owner, err := verifiedPlaceholderOwner(event)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	visible, err := event.Lstat(name)
	if err == nil {
		if !visible.Mode().IsRegular() || !os.SameFile(owner, visible) {
			return fmt.Errorf("visible placeholder is not owned by UFCM")
		}
		if err := event.Remove(name); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return event.Remove(placeholderOwner)
}
