// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package search

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strconv"
	"time"

	"github.com/autobrr/autobrr/internal/feed"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/pkg/errors"
)

// Candidate retains a short-lived, owner-bound NZB selection exclusively server-side.
type Candidate struct {
	URL                                                                   string `json:"-"`
	ID, Actor, EventID, Component, Title, Key, Indexer, Source, ConfigKey string
	FeedID                                                                int
	Size                                                                  uint64
	Expires                                                               time.Time
}

func (s *Service) remember(input Request, event catalogue.Event, source Source, item feed.SearchItem) string {
	u, err := url.Parse(item.URL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	ref := item.Reference
	if ref == "" {
		ref = item.URL
	}
	hash := sha256.Sum256([]byte(strconv.Itoa(source.ID) + "\x00" + ref))
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, c := range s.candidates {
		if now.After(c.Expires) {
			delete(s.candidates, id)
		}
	}
	if len(s.candidates) >= 1000 {
		oldest := ""
		var expiry time.Time
		for id, c := range s.candidates {
			if oldest == "" || c.Expires.Before(expiry) {
				oldest = id
				expiry = c.Expires
			}
		}
		delete(s.candidates, oldest)
	}
	id := rand.Text()
	s.candidates[id] = Candidate{ConfigKey: source.ConfigKey, ID: id, Actor: input.Actor, EventID: event.ID, Component: input.Component, Title: item.Title, URL: item.URL, Key: hex.EncodeToString(hash[:]), Indexer: source.Indexer, Source: source.Name, FeedID: source.ID, Size: item.Size, Expires: now.Add(15 * time.Minute)}
	return id
}

// CandidateFor returns only a still-valid candidate owned by this authenticated caller.
func (s *Service) CandidateFor(id, actor string) (Candidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.candidates[id]
	if !ok || c.Actor != actor || time.Now().After(c.Expires) {
		return Candidate{}, errors.Wrap(catalogue.ErrInvalidEdit, "search result expired or unavailable; run Manual Search again")
	}
	return c, nil
}

// AutomationActor owns server-selected feed candidates, independently of browser sessions.
const AutomationActor = "system:ufc-automation"

// RememberFeedCandidate uses the same bounded, server-side selection validation as manual search.
func (s *Service) RememberFeedCandidate(event, component string, source Source, item feed.SearchItem) string {
	return s.remember(Request{Actor: AutomationActor, Component: component}, catalogue.Event{ID: event}, source, item)
}
