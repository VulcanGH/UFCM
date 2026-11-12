// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package processing

import (
	"context"
	"sync"
	"time"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/downloader"
	"github.com/autobrr/autobrr/internal/maintenance"
	"github.com/autobrr/autobrr/internal/ufc/acquisition"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/library"
	"github.com/autobrr/autobrr/pkg/errors"
	"github.com/autobrr/autobrr/pkg/sabnzbd"
)

type repository interface {
	Acquisition(context.Context, string) (acquisition.Job, error)
	PendingProcessingHandoffs(context.Context) ([]acquisition.Job, error)
	FinishProcessingHandoff(context.Context, string) (bool, error)
	LoadCatalogueEvent(context.Context, string) (catalogue.Event, error)
	LibraryView(context.Context) (*library.View, error)
	CataloguePreferences(context.Context) (*catalogue.Settings, error)
	ProcessingSettings(context.Context) (*Settings, error)
	SaveProcessingSettings(context.Context, Settings) (*Settings, error)
	LibrarySettings(context.Context) (*library.Settings, error)
	EnqueueProcessingJob(context.Context, Job) (Job, error)
	ProcessingJob(context.Context, string) (Job, error)
	ProcessingJobs(context.Context) ([]Job, error)
	RecoverableProcessingJobs(context.Context) ([]Job, error)
	InterruptProcessingJobs(context.Context) error
	ClaimProcessingJob(context.Context) (Job, bool, error)
	SetProcessingFile(context.Context, string, string, string, string, string) error
	SetProcessingProgress(context.Context, string, string, int64) error
	ReviewProcessingJob(context.Context, string, string) error
	CompleteProcessingJob(context.Context, Job, int64, string, string) error
	ClaimProcessingCleanup(context.Context) (Job, bool, error)
	FinishProcessingCleanup(context.Context, string, string, string) error
	InterruptProcessingCleanup(context.Context) error
	RetryProcessingCleanup(context.Context, string) error
	RetryProcessingJob(context.Context, string) error
	ResolveProcessingJob(context.Context, string, string, string) error
	SetProcessingDestination(context.Context, string, string) error
	CancelRunningProcessingJob(context.Context, string) error
}

type downloaders interface {
	GetInstance(context.Context, int32) (*downloader.Instance, error)
}

// Service validates settings and lists existing SAB categories without processing media.
type Service struct {
	repo        repository
	downloaders downloaders
	wake        chan struct{}
	mu          sync.Mutex
	cancel      context.CancelFunc
	done        chan struct{}
	active      map[string]context.CancelFunc
	cancelled   map[string]bool
}

// New constructs the settings service without accessing roots or starting jobs.
func New(r repository, d downloaders) *Service {
	return &Service{repo: r, downloaders: d, wake: make(chan struct{}, 1), active: map[string]context.CancelFunc{}, cancelled: map[string]bool{}}
}

// CompletedDownload queues only an owned, configured and confidently previewed SAB completion.
func (s *Service) CompletedDownload(ctx context.Context, download acquisition.Job) error {
	ctx, release, ok := maintenance.Application.EnterContext(ctx)
	if !ok {
		return nil
	}
	defer release()
	settings, err := s.repo.ProcessingSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.AutoProcessCompleted || settings.SABClientID != download.ClientID || settings.SABCategory != download.Category {
		return nil
	}
	if done, err := s.repo.FinishProcessingHandoff(ctx, download.ID); err != nil || done {
		return err
	}
	_, err = s.Queue(ctx, download.ID)
	if err != nil {
		return err
	}
	_, err = s.repo.FinishProcessingHandoff(ctx, download.ID)
	return err
}

// Settings reads persistent processing preferences.
func (s *Service) Settings(ctx context.Context) (*Settings, error) {
	return s.repo.ProcessingSettings(ctx)
}

// SaveSettings validates paths and preferences without touching the filesystem.
func (s *Service) SaveSettings(ctx context.Context, input Settings) (*Settings, error) {
	librarySettings, err := s.repo.LibrarySettings(ctx)
	if err != nil {
		return nil, err
	}
	roots := []string{}
	destinations := []string{}
	for _, root := range librarySettings.Roots {
		roots = append(roots, root.Path)
		if root.Enabled {
			destinations = append(destinations, root.Path)
		}
	}
	if err := Validate(&input, roots, destinations); err != nil {
		return nil, err
	}
	if input.SABClientID != 0 {
		if _, err := s.sabClient(ctx, input.SABClientID); err != nil {
			return nil, err
		}
		if input.SABCategory != "" {
			previous, err := s.repo.ProcessingSettings(ctx)
			if err != nil {
				return nil, err
			}
			if previous.SABClientID != input.SABClientID || previous.SABCategory != input.SABCategory {
				categories, err := s.Categories(ctx, input.SABClientID)
				if err != nil {
					return nil, err
				}
				found := false
				for _, category := range categories {
					if category == input.SABCategory {
						found = true
						break
					}
				}
				if !found {
					return nil, errors.Wrap(catalogue.ErrInvalidEdit, "choose an existing SAB category")
				}
			}
		}
	} else if input.SABCategory != "" {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "choose a SAB client for this category")
	}
	return s.repo.SaveProcessingSettings(ctx, input)
}

// Categories lists categories from an existing enabled SAB client.
func (s *Service) Categories(ctx context.Context, id int32) ([]string, error) {
	client, err := s.sabClient(ctx, id)
	if err != nil {
		return nil, err
	}
	return client.Bounded(8 * time.Second).Categories(ctx)
}

func (s *Service) sabClient(ctx context.Context, id int32) (*sabnzbd.Client, error) {
	if id < 1 {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "choose a SAB client")
	}
	instance, err := s.downloaders.GetInstance(ctx, id)
	if err != nil || instance == nil {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "SAB client unavailable")
	}
	cfg := instance.Config()
	if cfg == nil || !cfg.Enabled || cfg.Type != domain.DownloaderTypeSabnzbd {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "choose an enabled SAB client")
	}
	client, err := instance.ClientAs[*sabnzbd.Client]()
	if err != nil {
		return nil, errors.Wrap(catalogue.ErrInvalidEdit, "SAB client unavailable")
	}
	return client, nil
}
