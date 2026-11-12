// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package acquisition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/downloader"
	"github.com/autobrr/autobrr/pkg/errors"
	"github.com/autobrr/autobrr/pkg/sabnzbd"
)

type downloaderService interface {
	List(context.Context) ([]domain.Downloader, error)
	GetInstance(context.Context, int32) (*downloader.Instance, error)
}
type remote interface {
	AddFromUrl(context.Context, sabnzbd.AddNzbRequest) (*sabnzbd.AddFileResponse, error)
	Lookup(context.Context, string, string) (sabnzbd.Job, error)
}
type target struct{ Name, Endpoint string }
type gateway interface {
	Clients(context.Context) ([]ClientOption, error)
	Target(context.Context, int32) (target, remote, error)
}

// Gateway uses existing downloader service configuration and cached SAB clients.
type Gateway struct{ downloaders downloaderService }

// NewGateway returns a gateway without creating connections or duplicating credentials.
func NewGateway(d downloaderService) *Gateway { return &Gateway{downloaders: d} }

// Clients lists only enabled SABnzbd client identities.
func (g *Gateway) Clients(ctx context.Context) ([]ClientOption, error) {
	clients, err := g.downloaders.List(ctx)
	if err != nil {
		return nil, errors.New("could not read configured download clients")
	}
	result := []ClientOption{}
	for _, c := range clients {
		if c.Enabled && c.Type == domain.DownloaderTypeSabnzbd {
			result = append(result, ClientOption{ID: c.ID, Name: c.Name})
		}
	}
	return result, nil
}

// Target resolves an existing enabled client and protects its endpoint identity.
func (g *Gateway) Target(ctx context.Context, id int32) (target, remote, error) {
	instance, err := g.downloaders.GetInstance(ctx, id)
	if err != nil || instance == nil {
		return target{}, nil, errors.New("configured SABnzbd client is unavailable")
	}
	cfg := instance.Config()
	if cfg == nil || !cfg.Enabled || cfg.Type != domain.DownloaderTypeSabnzbd {
		return target{}, nil, errors.New("configured SABnzbd client is disabled or incompatible")
	}
	client, err := instance.ClientAs[*sabnzbd.Client]()
	if err != nil {
		return target{}, nil, errors.New("configured SABnzbd client is unavailable")
	}
	hash := sha256.Sum256(fmt.Appendf(nil, "%s|%d|%t", cfg.Host, cfg.Port, cfg.TLS))
	return target{Name: cfg.Name, Endpoint: hex.EncodeToString(hash[:])}, client.Bounded(8 * time.Second), nil
}
