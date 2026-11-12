// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package feed

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/pkg/errors"
	"github.com/autobrr/autobrr/pkg/newznab"
	"github.com/autobrr/autobrr/pkg/torznab"

	"github.com/rs/zerolog"
)

// SearchItem contains preview metadata without credential-bearing links.
type SearchItem struct {
	PublishedAt *time.Time `json:"published_at"`
	URL         string     `json:"-"`
	Reference   string     `json:"-"`
	Title       string     `json:"title"`
	Size        uint64     `json:"size"`
}

// SearchPage bounds one manual search independently of scheduled feed processing.
type SearchPage struct {
	Items []SearchItem `json:"items"`
	More  bool         `json:"more"`
}

type searchBody struct {
	io.Reader
	io.Closer
}
type searchTransport struct {
	base              http.RoundTripper
	cookie, userAgent string
}

func (t searchTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	if t.cookie != "" {
		req.Header.Set("Cookie", t.cookie)
	}
	if t.userAgent != "" {
		req.Header.Set("User-Agent", t.userAgent)
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, errors.New("indexer request failed")
	}
	resp.Body = searchBody{Reader: io.LimitReader(resp.Body, 8<<20), Closer: resp.Body}
	return resp, nil
}

// Search queries one saved connection without invoking filters, actions or feed persistence.
func (s *Service) Search(ctx context.Context, id int, query string) (*SearchPage, error) {
	if id < 1 || len(query) > 160 || strings.TrimSpace(query) == "" {
		return nil, errors.New("invalid indexer search")
	}
	f, err := s.repo.FindByID(ctx, id)
	if err != nil || f == nil {
		return nil, errors.New("configured connection is unavailable")
	}
	if !f.Enabled || (f.Indexer.ID != 0 && !f.IndexerEnabled) {
		return nil, errors.New("configured connection is disabled")
	}
	if f.Type != string(domain.FeedTypeNewznab) && f.Type != string(domain.FeedTypeTorznab) {
		return nil, errors.New("connection does not support Newznab or Torznab search")
	}
	if err := f.Validate(); err != nil {
		return nil, errors.New("invalid connection settings")
	}
	if err := s.loadProxy(ctx, f); err != nil {
		return nil, errors.New("configured proxy is unavailable")
	}
	if f.UseProxy && (f.Proxy == nil || !f.Proxy.Enabled) {
		return nil, errors.New("configured proxy is disabled")
	}
	client, err := newHTTPClient(f)
	if err != nil {
		return nil, errors.New("could not initialise connection")
	}
	client.Timeout = min(client.Timeout, 8*time.Second)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client.Transport = searchTransport{base: client.Transport, cookie: f.Cookie, userAgent: f.UserAgent}
	page := &SearchPage{Items: []SearchItem{}}
	switch f.Type {
	case string(domain.FeedTypeNewznab):
		c := newznab.NewClient(newznab.Config{Host: f.URL, ApiKey: f.ApiKey, Log: zerolog.Nop()})
		c.WithHTTPClient(client)
		response, err := c.Search(ctx, query, f.Categories, 0)
		if err != nil {
			return nil, errors.New("indexer search failed; check connection and search support in existing settings")
		}
		for _, item := range response.Items {
			if item == nil {
				continue
			}
			size := item.Size
			if size == 0 && item.Enclosure != nil {
				size = item.Enclosure.Length
			}
			link := item.Link
			if item.Enclosure != nil && item.Enclosure.Type == "application/x-nzb" {
				link = item.Enclosure.Url
			}
			page.Items = append(page.Items, SearchItem{Title: item.Title, PublishedAt: publicationDate(item.PubDate.Time), Size: size, URL: link, Reference: item.GUID})
		}
		page.More = response.Total > len(response.Items) || len(response.Items) >= response.Limit
	case string(domain.FeedTypeTorznab):
		c := torznab.NewClient(torznab.Config{Host: f.URL, ApiKey: f.ApiKey, Log: zerolog.Nop()})
		c.WithHTTPClient(client)
		response, err := c.Search(ctx, query, f.Categories, 0)
		if err != nil {
			return nil, errors.New("indexer search failed; check connection and search support in existing settings")
		}
		for _, item := range response.Items {
			if item == nil {
				continue
			}
			size := item.Size
			if size == 0 && item.Enclosure != nil {
				if length, err := strconv.ParseUint(item.Enclosure.Length, 10, 64); err == nil {
					size = length
				}
			}
			page.Items = append(page.Items, SearchItem{Title: item.Title, PublishedAt: publicationDate(item.PubDate.Time), Size: size})
		}
		page.More = response.Total > len(response.Items) || len(response.Items) >= response.Limit
	}
	if len(page.Items) > 50 {
		page.Items = page.Items[:50]
		page.More = true
	}
	return page, nil
}

func publicationDate(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	value = value.UTC()
	return &value
}
