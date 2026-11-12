// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package sabnzbd

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/autobrr/autobrr/pkg/errors"
)

// Job identifies queue/history state without exposing remote errors or credentials.
type Job struct {
	ID, State, Storage, Title string
	Percent                   float64
	Found, Ambiguous, History bool
}
type jobSlot struct {
	ID       string          `json:"nzo_id"`
	Filename string          `json:"filename"`
	Name     string          `json:"name"`
	NZBName  string          `json:"nzb_name"`
	Status   string          `json:"status"`
	Storage  string          `json:"storage"`
	Percent  json.RawMessage `json:"percentage"`
}

func (c *Client) trackingSlots(ctx context.Context, mode, id, marker string, archive bool) ([]jobSlot, error) {
	v := url.Values{"mode": {mode}, "output": {"json"}, "apikey": {c.apiKey}, "limit": {"100"}}
	if id != "" {
		v.Set("nzo_ids", id)
	} else {
		v.Set("search", marker)
	}
	if archive {
		v.Set("archive", "1")
	}
	addr, err := url.JoinPath(c.addr, "api")
	if err != nil {
		return nil, errors.New("invalid SABnzbd address")
	}
	u, err := url.Parse(addr)
	if err != nil {
		return nil, errors.New("invalid SABnzbd address")
	}
	u.RawQuery = v.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New("could not create SABnzbd status request")
	}
	if c.basicUser != "" && c.basicPass != "" {
		req.SetBasicAuth(c.basicUser, c.basicPass)
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New("SABnzbd status request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("SABnzbd status request was rejected")
	}
	var result struct {
		Queue *struct {
			Slots []jobSlot `json:"slots"`
		} `json:"queue"`
		History *struct {
			Slots []jobSlot `json:"slots"`
		} `json:"history"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return nil, errors.New("invalid SABnzbd status response")
	}
	if mode == "queue" && result.Queue != nil {
		return result.Queue.Slots, nil
	}
	if mode == "history" && result.History != nil {
		return result.History.Slots, nil
	}
	return nil, errors.New("SABnzbd status is unavailable")
}

// Lookup checks queue and history by remote ID or an application-owned job marker.
func (c *Client) Lookup(ctx context.Context, id, marker string) (Job, error) {
	if id == "" && marker == "" {
		return Job{}, errors.New("job identity is required")
	}
	for _, phase := range []struct {
		mode    string
		archive bool
	}{{"queue", false}, {"history", false}, {"history", true}} {
		slots, err := c.trackingSlots(ctx, phase.mode, id, marker, phase.archive)
		if err != nil {
			return Job{}, err
		}
		found := []jobSlot{}
		for _, slot := range slots {
			name := slot.Filename
			if name == "" {
				name = slot.Name
			}
			if name == "" {
				name = slot.NZBName
			}
			name = strings.TrimSpace(name)
			if len(name) >= 4 && strings.EqualFold(name[len(name)-4:], ".nzb") {
				name = name[:len(name)-4]
			}
			if slot.ID != "" && ((id != "" && slot.ID == id) || (id == "" && (strings.HasPrefix(name, marker+" ") || strings.HasSuffix(name, " "+marker)))) {
				found = append(found, slot)
			}
		}
		if len(found) > 1 {
			return Job{Ambiguous: true}, nil
		}
		if len(found) == 1 {
			slot := found[0]
			percent, _ := strconv.ParseFloat(strings.Trim(string(slot.Percent), `"`), 64)
			name := slot.Filename
			if name == "" {
				name = slot.Name
			}
			if name == "" {
				name = slot.NZBName
			}
			return Job{Title: name, Found: true, History: phase.mode == "history", ID: slot.ID, State: slot.Status, Storage: slot.Storage, Percent: percent}, nil
		}
	}
	return Job{}, nil
}
