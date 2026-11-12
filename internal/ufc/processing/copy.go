// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package processing

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/autobrr/autobrr/internal/maintenance"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/pkg/errors"
)

// Queue persists a review-free preview for an explicit copy.
func (s *Service) Queue(ctx context.Context, acquisitionID string) (Job, error) {
	preview, err := s.Preview(ctx, acquisitionID)
	if err != nil {
		return Job{}, err
	}
	if len(preview.Files) == 0 || len(preview.Files) > 8 || preview.Status != "ready" && !conflictOnly(preview) {
		return Job{}, errors.Wrap(catalogue.ErrInvalidEdit, "media files need a clear match and no more than eight files; review the preview first")
	}
	acquisition, err := s.repo.Acquisition(ctx, acquisitionID)
	if err != nil {
		return Job{}, err
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return Job{}, err
	}
	id := hex.EncodeToString(random[:])
	now := time.Now().UTC().Format(time.RFC3339Nano)
	settings, err := s.repo.ProcessingSettings(ctx)
	if err != nil {
		return Job{}, err
	}
	job := Job{ID: id, AcquisitionID: acquisitionID, EventID: preview.EventID, Component: acquisition.Component, DestinationRoot: preview.DestinationRoot, State: "queued", CreatedAt: now, UpdatedAt: now, CleanupRequested: settings.CleanupSABDownload, Files: make([]JobFile, 0, len(preview.Files))}
	for index, file := range preview.Files {
		stageName := ".ufcm-" + id + ".part"
		if len(preview.Files) > 1 {
			stageName = ".ufcm-" + id + "-" + strconv.Itoa(index+1) + ".part"
		}
		stage := filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(file.Destination)), stageName))
		job.Files = append(job.Files, JobFile{Source: file.Source, Component: file.Component, Destination: file.Destination, OriginalDestination: file.Destination, Stage: stage, Size: file.Size, State: "queued"})
	}
	if preview.Status != "ready" {
		job.State = "needs_review"
		job.Message = "Existing destination needs an explicit skip or keep-both choice"
	}
	stored, err := s.repo.EnqueueProcessingJob(ctx, job)
	if err != nil {
		return Job{}, err
	}
	if stored.State == "queued" {
		s.signal()
	}
	return stored, nil
}

// Job reads one persistent copy job.
func (s *Service) Job(ctx context.Context, id string) (Job, error) {
	return s.repo.ProcessingJob(ctx, id)
}

// Jobs reads recent persistent copy jobs.
func (s *Service) Jobs(ctx context.Context) ([]Job, error) {
	return s.repo.ProcessingJobs(ctx)
}

// RetryCleanup rechecks one completed, reviewed job without copying any files.
func (s *Service) RetryCleanup(ctx context.Context, id string) (Job, error) {
	if id == "" || len(id) > 128 {
		return Job{}, errors.Wrap(catalogue.ErrInvalidEdit, "invalid copy job")
	}
	settings, err := s.repo.ProcessingSettings(ctx)
	if err != nil {
		return Job{}, err
	}
	if !settings.CleanupSABDownload {
		return Job{}, errors.Wrap(catalogue.ErrInvalidEdit, "enable source cleanup in UFC settings first")
	}
	if err := s.repo.RetryProcessingCleanup(ctx, id); err != nil {
		return Job{}, err
	}
	s.signal()
	return s.repo.ProcessingJob(ctx, id)
}

func (s *Service) signal() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Start starts one bounded worker; queued jobs are the only work it may claim.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return nil
	}
	if err := s.repo.InterruptProcessingJobs(ctx); err != nil {
		return err
	}
	if err := s.repo.InterruptProcessingCleanup(ctx); err != nil {
		return err
	}
	settings, err := s.repo.ProcessingSettings(ctx)
	if err != nil {
		return err
	}
	if settings.MaxConcurrent < 1 || settings.MaxConcurrent > 4 {
		return errors.Wrap(catalogue.ErrInvalidEdit, "invalid processing worker limit")
	}
	workerCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.done = make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		_ = s.reconcileInterrupted(workerCtx)
	}()
	workers.Add(1)
	go func() {
		defer workers.Done()
		s.reconcileHandoffs(workerCtx)
	}()
	for range settings.MaxConcurrent {
		workers.Add(1)
		go func() {
			defer workers.Done()
			s.run(workerCtx)
		}()
	}
	go func() {
		workers.Wait()
		close(s.done)
	}()
	return nil
}

func (s *Service) reconcileHandoffs(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		jobs, err := s.repo.PendingProcessingHandoffs(ctx)
		if err == nil {
			for _, job := range jobs {
				if ctx.Err() != nil {
					return
				}
				_ = s.CompletedDownload(ctx, job)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Stop cancels the worker and waits for its current safe checkpoint.
func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Service) run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		workCtx, release, allowed := maintenance.Application.EnterContext(ctx)
		if !allowed {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				continue
			}
		}
		if cleanup, ok, err := s.repo.ClaimProcessingCleanup(workCtx); err == nil && ok {
			message := s.cleanupCompleted(workCtx, cleanup)
			state := "done"
			if message != "" {
				state = "needs_review"
			}
			saveCtx, cancel := context.WithTimeout(context.WithoutCancel(workCtx), 5*time.Second)
			_ = s.repo.FinishProcessingCleanup(saveCtx, cleanup.ID, state, message)
			cancel()
			release()
			continue
		}
		s.mu.Lock()
		job, ok, err := s.repo.ClaimProcessingJob(workCtx)
		var jobCtx context.Context
		var cancel context.CancelFunc
		if err == nil && ok {
			jobCtx, cancel = context.WithCancel(workCtx)
			s.active[job.ID] = cancel
		}
		s.mu.Unlock()
		if err == nil && ok {
			message := s.copyJob(jobCtx, job)
			s.mu.Lock()
			cancelled := s.cancelled[job.ID]
			delete(s.active, job.ID)
			delete(s.cancelled, job.ID)
			s.mu.Unlock()
			cancel()
			if message != "" {
				saveCtx, cancel := context.WithTimeout(context.WithoutCancel(workCtx), 5*time.Second)
				if cancelled {
					_ = s.repo.CancelRunningProcessingJob(saveCtx, job.ID)
				} else {
					_ = s.repo.ReviewProcessingJob(saveCtx, job.ID, message)
				}
				cancel()
			}
			release()
			continue
		}
		release()
		select {
		case <-ctx.Done():
			return
		case <-s.wake:
		case <-ticker.C:
		}
	}
}

func (s *Service) copyJob(ctx context.Context, job Job) string {
	if len(job.Files) == 0 || len(job.Files) > 8 {
		return "Media file count needs review"
	}
	plan, err := s.Preview(ctx, job.AcquisitionID)
	if err != nil || !planMatchesJob(plan, job) {
		return "Source, catalogue, Library or destination changed since preview; review before copying"
	}
	for _, file := range job.Files {
		if file.State == "verified" {
			continue
		}
		one := job
		one.Files = []JobFile{file}
		if message := s.copyOne(ctx, one); message != "" {
			return file.Source + ": " + message
		}
	}
	return ""
}

func (s *Service) copyOne(ctx context.Context, job Job) string {
	acquisition, err := s.repo.Acquisition(ctx, job.AcquisitionID)
	if err != nil || acquisition.State != "completed" || acquisition.Storage == "" {
		return "Completed SAB download is no longer confirmed"
	}
	storageInfo, err := os.Lstat(acquisition.Storage)
	if err != nil || storageInfo.Mode()&os.ModeSymlink != 0 {
		return "Completed SAB storage is unavailable or linked"
	}
	source := acquisition.Storage
	if storageInfo.IsDir() {
		source = filepath.Join(source, filepath.FromSlash(job.Files[0].Source))
	}
	resolvedSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		return "Source path changed or escaped completed SAB storage"
	}
	resolvedStorage, err := filepath.EvalSymlinks(acquisition.Storage)
	if err != nil || storageInfo.IsDir() && !within(resolvedStorage, resolvedSource) {
		return "Source path changed or escaped completed SAB storage"
	}
	before, err := os.Lstat(source)
	if err != nil || !before.Mode().IsRegular() || before.Size() != job.Files[0].Size {
		return "Source media changed since preview"
	}
	input, err := os.Open(source)
	if err != nil {
		return "Source media could not be opened"
	}
	defer input.Close()
	opened, err := input.Stat()
	if err != nil || !os.SameFile(before, opened) {
		return "Source media changed while opening"
	}
	file := job.Files[0]
	if err := safeDestinationDirectory(job.DestinationRoot, filepath.Dir(filepath.FromSlash(file.Destination))); err != nil {
		return "Library destination is unavailable or unsafe"
	}
	final := filepath.Join(job.DestinationRoot, filepath.FromSlash(file.Destination))
	stage := filepath.Join(job.DestinationRoot, filepath.FromSlash(file.Stage))
	if destinationState(job.DestinationRoot, file.Destination) != "planned" {
		return "Destination or job staging path already exists; review the Library"
	}
	if stageInfo, err := os.Lstat(stage); err == nil {
		if !stageInfo.Mode().IsRegular() || stageInfo.Size() != file.Size {
			return "Staging file changed; manual review required"
		}
		sourceHash := sha256.New()
		if _, err := io.Copy(sourceHash, input); err != nil {
			return "Source could not be verified against staging"
		}
		if after, err := input.Stat(); err != nil || !os.SameFile(before, after) || !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
			return "Source changed while verifying staging"
		}
		hash := hex.EncodeToString(sourceHash.Sum(nil))
		stagedHash, _, err := hashFile(ctx, stage)
		if err != nil || stagedHash != hash {
			return "Staging bytes differ from source; manual review required"
		}
		return s.publishStage(ctx, job, stage, final, hash, file.Size)
	} else if !os.IsNotExist(err) {
		return "Staging path cannot be inspected"
	}
	if err := s.repo.SetProcessingFile(ctx, job.ID, file.Source, "staging", "", ""); err != nil {
		return "Could not record copy checkpoint"
	}
	output, err := os.OpenFile(stage, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "Could not create a private staging file"
	}
	sourceHash := sha256.New()
	buffer := make([]byte, 1024*1024)
	var copied int64
	for {
		if ctx.Err() != nil {
			output.Close()
			return "Copy interrupted; staging file retained for review"
		}
		n, readErr := input.Read(buffer)
		if n > 0 {
			if _, err := sourceHash.Write(buffer[:n]); err != nil {
				output.Close()
				return "Source hash failed; staging file retained"
			}
			written, writeErr := output.Write(buffer[:n])
			copied += int64(written)
			if writeErr != nil || written != n {
				output.Close()
				return "Staging write failed; staging file retained"
			}
			if copied == int64(written) || copied%(16*1024*1024) < int64(written) {
				if err := s.repo.SetProcessingProgress(ctx, job.ID, file.Source, copied); err != nil {
					output.Close()
					return "Could not record copy progress; staging file retained"
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			output.Close()
			return "Source read failed; staging file retained"
		}
	}
	if err := output.Sync(); err != nil {
		output.Close()
		return "Staging flush failed; staging file retained"
	}
	if err := output.Close(); err != nil {
		return "Staging close failed; staging file retained"
	}
	after, err := input.Stat()
	if err != nil || !os.SameFile(before, after) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || copied != before.Size() {
		return "Source media changed during copy; staging file retained"
	}
	hash := hex.EncodeToString(sourceHash.Sum(nil))
	stagedHash, _, err := hashFile(ctx, stage)
	if err != nil || stagedHash != hash {
		return "Staged copy failed hash verification; staging file retained"
	}
	return s.publishStage(ctx, job, stage, final, hash, copied)
}

func (s *Service) publishStage(ctx context.Context, job Job, stage, final, hash string, copied int64) string {
	file := job.Files[0]
	if ctx.Err() != nil {
		return "Copy interrupted; staging file retained for review"
	}
	if err := os.Link(stage, final); err != nil {
		if os.IsExist(err) {
			return "Destination appeared during copy; existing media was preserved"
		}
		return "Could not publish copy without replacement; staging file retained"
	}
	if err := s.repo.SetProcessingFile(ctx, job.ID, file.Source, "published", hash, ""); err != nil {
		return "Copy was published but journal update failed; review Library"
	}
	stageInfo, stageErr := os.Lstat(stage)
	finalInfo, finalErr := os.Lstat(final)
	if stageErr != nil || finalErr != nil || !stageInfo.Mode().IsRegular() || !finalInfo.Mode().IsRegular() || !os.SameFile(stageInfo, finalInfo) || !ownedStageName(job.ID, filepath.Base(stage)) {
		return "Published copy and staging ownership need review"
	}
	if err := os.Remove(stage); err != nil {
		return "Copy was published but private staging cleanup failed; review Library"
	}
	finalHash, finalInfo, err := hashFile(ctx, final)
	if err != nil || finalHash != hash || finalInfo.Size() != copied {
		return "Published copy failed final verification; review Library"
	}
	if err := s.repo.CompleteProcessingJob(ctx, job, copied, finalInfo.ModTime().UTC().Format(time.RFC3339Nano), hash); err != nil {
		return "Verified copy was published but Library record failed; review before retrying"
	}
	return ""
}

func ownedStageName(id, name string) bool {
	return name == ".ufcm-"+id+".part" || strings.HasPrefix(name, ".ufcm-"+id+"-") && strings.HasSuffix(name, ".part")
}

func conflictOnly(plan *Preview) bool {
	if plan == nil || len(plan.Files) != 1 || len(plan.Files[0].Reasons) != 1 || plan.Files[0].Reasons[0] != "Destination exists or contains an unsafe path; content is not verified" {
		return false
	}
	for _, reason := range plan.Reasons {
		if reason != "Existing library files for this event need folder review" {
			return false
		}
	}
	return true
}

func planMatchesJob(plan *Preview, job Job) bool {
	if plan == nil || len(plan.Files) != len(job.Files) || len(job.Files) == 0 || plan.EventID != job.EventID || plan.DestinationRoot != job.DestinationRoot {
		return false
	}
	if len(job.Files) > 1 {
		verified := false
		bySource := make(map[string]FilePlan, len(plan.Files))
		for _, file := range plan.Files {
			bySource[file.Source] = file
		}
		for _, file := range job.Files {
			proposed, ok := bySource[file.Source]
			if !ok || proposed.Size != file.Size || proposed.Component != file.Component || proposed.Destination != originalDestination(file) {
				return false
			}
			if file.State == "verified" {
				verified = true
				continue
			}
			if proposed.State != "planned" || len(proposed.Reasons) > 0 {
				return false
			}
		}
		for _, reason := range plan.Reasons {
			if !verified || reason != "Existing library files for this event need folder review" {
				return false
			}
		}
		return true
	}
	if plan.Files[0].Source != job.Files[0].Source || plan.Files[0].Size != job.Files[0].Size {
		return false
	}
	original := job.Files[0].OriginalDestination
	if original == "" {
		original = job.Files[0].Destination
	}
	if plan.Files[0].Destination != original {
		return false
	}
	if job.Files[0].Destination == original {
		return plan.Status == "ready"
	}
	return conflictOnly(plan) && destinationState(job.DestinationRoot, job.Files[0].Destination) == "planned"
}

func safeDestinationDirectory(root, relative string) error {
	if relative == "." || !within(root, filepath.Join(root, relative)) || filepath.IsAbs(relative) {
		return fmt.Errorf("invalid destination directory")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("invalid destination root")
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("invalid destination component")
		}
		current = filepath.Join(current, part)
		if err := os.Mkdir(current, 0755); err != nil && !os.IsExist(err) {
			return err
		}
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("destination contains a linked or non-directory path")
		}
		resolved, err := filepath.EvalSymlinks(current)
		if err != nil || !within(resolvedRoot, resolved) {
			return fmt.Errorf("destination escapes library root")
		}
	}
	return nil
}

func hashFile(ctx context.Context, path string) (string, os.FileInfo, error) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return "", nil, fmt.Errorf("not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(before, info) {
		return "", nil, fmt.Errorf("not a regular file")
	}
	hash := sha256.New()
	buffer := make([]byte, 1024*1024)
	for {
		if err := ctx.Err(); err != nil {
			return "", nil, err
		}
		n, err := file.Read(buffer)
		if n > 0 {
			_, _ = hash.Write(buffer[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", nil, err
		}
	}
	after, err := file.Stat()
	if err != nil || !os.SameFile(info, after) || info.Size() != after.Size() || !info.ModTime().Equal(after.ModTime()) {
		return "", nil, fmt.Errorf("file changed during verification")
	}
	return hex.EncodeToString(hash.Sum(nil)), info, nil
}
