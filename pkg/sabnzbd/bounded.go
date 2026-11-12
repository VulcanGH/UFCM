// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package sabnzbd

import (
	"io"
	"net/http"
	"time"
)

type boundedBody struct {
	io.Reader
	io.Closer
}
type boundedTransport struct{ base http.RoundTripper }

func (t boundedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	resp.Body = boundedBody{Reader: io.LimitReader(resp.Body, 4<<20), Closer: resp.Body}
	return resp, nil
}

// Bounded returns an independent client with bounded reads/timeouts and no redirects.
func (c *Client) Bounded(timeout time.Duration) *Client {
	clone := *c
	client := *c.http
	client.Timeout = timeout
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client.Transport = boundedTransport{base: transport}
	clone.http = &client
	return &clone
}
