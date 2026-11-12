// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package processing

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/autobrr/autobrr/internal/maintenance"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/pkg/errors"
)

// ReviewOptions describes explicit recovery choices without modifying media.
type ReviewOptions struct {
	Retry               bool   `json:"retry"`
	Skip                bool   `json:"skip"`
	Cancel              bool   `json:"cancel"`
	KeepBothDestination string `json:"keep_both_destination"`
	Preferred           string `json:"preferred"`
	Reason              string `json:"reason"`
}

// Review reads current paths and offers only choices safe for this job state.
func (s *Service) Review(ctx context.Context, id string) (ReviewOptions, error) {
	job, err := s.repo.ProcessingJob(ctx, id)
	if err != nil {
		return ReviewOptions{}, err
	}
	options := ReviewOptions{}
	if settings, err := s.repo.ProcessingSettings(ctx); err == nil {
		options.Preferred = settings.ConflictPolicy
	}
	if len(job.Files) == 0 || job.Resolution != "" || job.State == "completed" {
		return options, nil
	}
	if job.State == "running" {
		options.Cancel = true
		options.Reason = "The worker will stop at its next copy checkpoint."
		return options, nil
	}
	if len(job.Files) > 1 {
		fresh := true
		skippable := job.State == "needs_review"
		for _, file := range job.Files {
			if file.State == "verified" {
				fresh = false
				continue
			}
			stage, ok := jobPath(job, file.Stage)
			if !ok || file.State != "queued" {
				fresh = false
				skippable = false
				continue
			}
			if _, err := os.Lstat(stage); !os.IsNotExist(err) {
				fresh = false
				skippable = false
			}
		}
		options.Cancel = job.State == "queued" && fresh
		options.Skip = skippable
		options.Retry = job.State == "needs_review"
		options.Reason = "Each file keeps its own verified state. Retry checks the unfinished files again."
		return options, nil
	}
	file := job.Files[0]
	stage, ok := jobPath(job, file.Stage)
	if !ok {
		return options, nil
	}
	_, stageErr := os.Lstat(stage)
	stageAbsent := os.IsNotExist(stageErr)
	if job.State != "queued" && job.State != "needs_review" {
		return options, nil
	}
	options.Cancel = job.State == "queued" && stageAbsent && file.State == "queued"
	options.Skip = job.State == "needs_review" && stageAbsent && file.State == "queued"
	options.Retry = job.State == "needs_review"
	if final, ok := jobPath(job, file.Destination); ok {
		if _, err := os.Lstat(final); os.IsNotExist(err) && file.State == "staging" && stageAbsent && job.State == "needs_review" {
			options.Skip = true
		} else if err == nil && file.State == "queued" {
			options.Retry = false
			options.Reason = "The existing destination will not be replaced. Choose keep both or skip."
		}
	}
	if options.Skip {
		plan, err := s.Preview(ctx, job.AcquisitionID)
		if err == nil && conflictOnly(plan) && file.Destination == originalDestination(file) {
			options.KeepBothDestination = keepBothPath(job.DestinationRoot, file.Destination)
		}
	}
	if !stageAbsent {
		options.Reason = "A job-owned staging file exists. Retry will verify it against the source before publication."
	} else if file.State == "published" {
		options.Reason = "A published file needs hash and Library reconciliation."
	}
	return options, nil
}

// Retry rechecks an unresolved job and resumes only safe, explicit work.
func (s *Service) Retry(ctx context.Context, id string) (Job, error) {
	job, err := s.repo.ProcessingJob(ctx, id)
	if err != nil {
		return Job{}, err
	}
	if job.State != "needs_review" || job.Resolution != "" || len(job.Files) == 0 {
		return Job{}, errors.Wrap(catalogue.ErrRevisionConflict, "job is not available for retry")
	}
	if len(job.Files) > 1 {
		for _, file := range job.Files {
			if file.State != "published" {
				continue
			}
			one := job
			one.Files = []JobFile{file}
			_ = s.reconcileOne(ctx, one)
		}
		job, err = s.repo.ProcessingJob(ctx, id)
		if err != nil || job.State == "completed" {
			return job, err
		}
		for _, file := range job.Files {
			if file.State == "verified" {
				continue
			}
			final, ok := jobPath(job, file.Destination)
			if !ok {
				return Job{}, errors.Wrap(catalogue.ErrInvalidEdit, "saved destination is unsafe")
			}
			if _, err := os.Lstat(final); err == nil {
				return Job{}, errors.Wrap(catalogue.ErrInvalidEdit, "unfinished file already exists; review its saved evidence")
			} else if !os.IsNotExist(err) {
				return Job{}, err
			}
		}
		plan, err := s.Preview(ctx, job.AcquisitionID)
		if err != nil || !planMatchesJob(plan, job) {
			return Job{}, errors.Wrap(catalogue.ErrInvalidEdit, "bundle changed; refresh the preview before retrying")
		}
		if err := s.repo.RetryProcessingJob(ctx, id); err != nil {
			return Job{}, err
		}
		s.signal()
		return s.repo.ProcessingJob(ctx, id)
	}
	if job.Files[0].State == "staging" || job.Files[0].State == "published" {
		if s.reconcileOne(ctx, job) {
			return s.repo.ProcessingJob(ctx, id)
		}
	}
	final, ok := jobPath(job, job.Files[0].Destination)
	if !ok {
		return Job{}, errors.Wrap(catalogue.ErrInvalidEdit, "saved destination is unsafe")
	}
	if _, err := os.Lstat(final); err == nil {
		return Job{}, errors.Wrap(catalogue.ErrInvalidEdit, "destination exists; choose keep both or review the published copy")
	} else if !os.IsNotExist(err) {
		return Job{}, err
	}
	plan, err := s.Preview(ctx, job.AcquisitionID)
	if err != nil || !planMatchesJob(plan, job) {
		return Job{}, errors.Wrap(catalogue.ErrInvalidEdit, "source or destination changed; refresh the preview before retrying")
	}
	if err := s.repo.RetryProcessingJob(ctx, id); err != nil {
		return Job{}, err
	}
	s.signal()
	return s.repo.ProcessingJob(ctx, id)
}

// KeepBoth records one specific, available alternate filename before retrying.
func (s *Service) KeepBoth(ctx context.Context, id, destination string) (Job, error) {
	job, err := s.repo.ProcessingJob(ctx, id)
	if err != nil {
		return Job{}, err
	}
	if job.State != "needs_review" || job.Resolution != "" || len(job.Files) != 1 || job.Files[0].State != "queued" {
		return Job{}, errors.Wrap(catalogue.ErrRevisionConflict, "job cannot use keep both")
	}
	options, err := s.Review(ctx, id)
	if err != nil {
		return Job{}, err
	}
	if options.KeepBothDestination == "" || destination != options.KeepBothDestination || destinationState(job.DestinationRoot, destination) != "planned" {
		return Job{}, errors.Wrap(catalogue.ErrInvalidEdit, "alternate destination changed; review the new suggestion")
	}
	if err := s.repo.SetProcessingDestination(ctx, id, destination); err != nil {
		return Job{}, err
	}
	s.signal()
	return s.repo.ProcessingJob(ctx, id)
}

// Skip resolves a reviewed job without creating or deleting media.
func (s *Service) Skip(ctx context.Context, id string) (Job, error) {
	options, err := s.Review(ctx, id)
	if err != nil {
		return Job{}, err
	}
	if !options.Skip {
		return Job{}, errors.Wrap(catalogue.ErrInvalidEdit, "job has staged or published media; skip is unavailable")
	}
	if err := s.repo.ResolveProcessingJob(ctx, id, "skipped", "Skipped by user; source and Library files unchanged"); err != nil {
		return Job{}, err
	}
	return s.repo.ProcessingJob(ctx, id)
}

// Cancel stops queued work or asks a running worker to stop at a safe checkpoint.
func (s *Service) Cancel(ctx context.Context, id string) (Job, error) {
	s.mu.Lock()
	if stop := s.active[id]; stop != nil {
		s.cancelled[id] = true
		stop()
		s.mu.Unlock()
		return s.repo.ProcessingJob(ctx, id)
	}
	s.mu.Unlock()
	options, err := s.Review(ctx, id)
	if err != nil {
		return Job{}, err
	}
	if !options.Cancel {
		return Job{}, errors.Wrap(catalogue.ErrRevisionConflict, "job is no longer queued or has staged media")
	}
	if err := s.repo.ResolveProcessingJob(ctx, id, "cancelled", "Cancelled before copying; source and Library files unchanged"); err != nil {
		return Job{}, err
	}
	return s.repo.ProcessingJob(ctx, id)
}

func (s *Service) reconcileInterrupted(ctx context.Context) error {
	ctx, release, ok := maintenance.Application.EnterContext(ctx)
	if !ok {
		return nil
	}
	defer release()
	jobs, err := s.repo.RecoverableProcessingJobs(ctx)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		for _, file := range job.Files {
			if file.State != "published" {
				continue
			}
			one := job
			one.Files = []JobFile{file}
			_ = s.reconcileOne(ctx, one)
		}
	}
	return nil
}

func (s *Service) reconcileOne(ctx context.Context, job Job) bool {
	if len(job.Files) != 1 || job.Resolution != "" || job.State != "needs_review" {
		return false
	}
	file := job.Files[0]
	if file.State != "staging" && file.State != "published" {
		return false
	}
	final, ok := jobPath(job, file.Destination)
	if !ok || !safeExistingFile(job.DestinationRoot, file.Destination) {
		return false
	}
	hash, info, err := hashFile(ctx, final)
	if err != nil || info.Size() != file.Size {
		return false
	}
	expected := file.SourceSHA256
	if expected == "" {
		plan, err := s.Preview(ctx, job.AcquisitionID)
		if err != nil || !conflictOnly(plan) || plan.EventID != job.EventID || len(plan.Files) != 1 || plan.Files[0].Source != file.Source || plan.Files[0].Size != file.Size {
			return false
		}
		acquisition, err := s.repo.Acquisition(ctx, job.AcquisitionID)
		if err != nil || acquisition.Storage == "" {
			return false
		}
		source := acquisition.Storage
		storage, err := os.Lstat(source)
		if err != nil || storage.Mode()&os.ModeSymlink != 0 {
			return false
		}
		if storage.IsDir() {
			source = filepath.Join(source, filepath.FromSlash(file.Source))
			if !safeExistingFile(acquisition.Storage, file.Source) {
				return false
			}
		} else if !storage.Mode().IsRegular() || filepath.Base(source) != file.Source {
			return false
		}
		expected, _, err = hashFile(ctx, source)
		if err != nil {
			return false
		}
	}
	if expected != hash {
		return false
	}
	stage, ok := jobPath(job, file.Stage)
	if !ok {
		return false
	}
	if stageInfo, err := os.Lstat(stage); err == nil {
		finalInfo, err := os.Lstat(final)
		if err != nil || !stageInfo.Mode().IsRegular() || !os.SameFile(stageInfo, finalInfo) || !ownedStageName(job.ID, filepath.Base(stage)) {
			return false
		}
		if err := os.Remove(stage); err != nil {
			return false
		}
	} else if !os.IsNotExist(err) {
		return false
	} else if file.State != "published" {
		return false
	}
	if err := s.repo.SetProcessingFile(ctx, job.ID, file.Source, "published", hash, ""); err != nil {
		return false
	}
	return s.repo.CompleteProcessingJob(ctx, job, info.Size(), info.ModTime().UTC().Format(time.RFC3339Nano), hash) == nil
}

func originalDestination(file JobFile) string {
	if file.OriginalDestination != "" {
		return file.OriginalDestination
	}
	return file.Destination
}

func keepBothPath(root, original string) string {
	ext := filepath.Ext(original)
	stem := strings.TrimSuffix(original, ext)
	for number := 2; number <= 99; number++ {
		candidate := stem + " (copy " + strconv.Itoa(number) + ")" + ext
		if destinationState(root, candidate) == "planned" {
			return candidate
		}
	}
	return ""
}

func jobPath(job Job, relative string) (string, bool) {
	if relative == "" || filepath.IsAbs(relative) || strings.Contains(relative, "\\") {
		return "", false
	}
	for _, part := range strings.Split(relative, "/") {
		if part == "" || part == "." || part == ".." {
			return "", false
		}
	}
	path := filepath.Join(job.DestinationRoot, filepath.FromSlash(relative))
	return path, within(job.DestinationRoot, path)
}

func safeExistingFile(root, relative string) bool {
	job := Job{DestinationRoot: root}
	path, ok := jobPath(job, relative)
	if !ok {
		return false
	}
	current := root
	rootInfo, err := os.Lstat(current)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return false
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, root+string(filepath.Separator)), string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return false
		}
	}
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
}
