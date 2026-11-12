// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package sabnzbd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"

	"github.com/autobrr/autobrr/pkg/errors"
	"github.com/autobrr/autobrr/pkg/sharedhttp"
)

// Categories lists configured SABnzbd categories without changing its configuration.
func (c *Client) Categories(ctx context.Context) ([]string, error) {
	addr, err := url.JoinPath(c.addr, "/api")
	if err != nil {
		return nil, errors.New("invalid SABnzbd address")
	}
	u, err := url.Parse(addr)
	if err != nil {
		return nil, errors.New("invalid SABnzbd address")
	}
	query := url.Values{"mode": {"get_cats"}, "output": {"json"}, "apikey": {c.apiKey}}
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New("could not create SABnzbd category request")
	}
	if c.basicUser != "" && c.basicPass != "" {
		req.SetBasicAuth(c.basicUser, c.basicPass)
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, errors.New("SABnzbd categories unavailable")
	}
	defer sharedhttp.DrainAndClose(resp)
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("SABnzbd categories unavailable")
	}
	var result struct {
		Categories []string `json:"categories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.New("invalid SABnzbd categories response")
	}
	values := []string{}
	seen := map[string]bool{}
	for _, category := range result.Categories {
		if category != "" && category != "*" && len(category) <= 128 && !seen[category] {
			values = append(values, category)
			seen[category] = true
		}
	}
	sort.Strings(values)
	return values, nil
}
