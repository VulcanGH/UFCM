// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func listHTML(scheduled, past string) string {
	return `<table><tr><th>Unrelated</th></tr></table><h2>Scheduled events</h2><table><tr><th>Date</th><th>Event</th></tr>` + scheduled + `</table><h2>Past events</h2><table><tr><th>#</th><th>Event</th><th>Date</th><th>Attendance</th></tr>` + past + `</table>`
}

func TestSemanticTablesAndMergedCells(t *testing.T) {
	rows, err := parseList(listHTML(`<tr><td rowspan="2">Jan 23, 2027<sup>[1]</sup></td><td><a href="/wiki/UFC_337">UFC 337: José vs. Procházka 2</a></td></tr><tr><td>UFC Fight Night TBA</td></tr>`, `<tr><td>123</td><td><a href="/wiki/UFC_37.5">UFC 37.5: As Real As It Gets</a></td><td>Jun 22, 2002</td><td>1,000</td></tr><tr><td colspan="2">UFC cancelled</td><td>TBA</td><td>Canceled</td></tr>`), 1)
	require.NoError(t, err)
	require.Len(t, rows, 4)
	require.Equal(t, "2027-01-23", *rows[1].Date)
	require.Equal(t, []string{"José", "Procházka"}, rows[0].Headliners)
	require.Equal(t, "123", rows[2].OverallSequence)
	require.Equal(t, "37.5", rows[2].Aliases[1].Value)
	require.True(t, rows[1].Blocked)
	require.Equal(t, "cancelled", rows[3].Status)
	require.Nil(t, rows[3].Date)
	require.Equal(t, "unknown", rows[2].Status)
	for _, bad := range []string{`<h2>Scheduled events</h2><table><tr><th>Event</th><th>Date</th></tr></table>`, strings.ReplaceAll(listHTML("", ""), "<th>Event</th>", "<th>Renamed</th>")} {
		_, err := parseList(bad, 1)
		require.Error(t, err)
	}
}

func TestMalformedPinnedRowspanUsesHTMLIntegerRules(t *testing.T) {
	rows, err := parseList(listHTML(`<tr><td rowspan='2data-sort-value=""'>Jan 23, 2027</td><td><a href="/wiki/UFC_337">UFC 337</a></td></tr><tr><td><a href="/wiki/UFC_Fight_Night_300">UFC Fight Night 300</a></td></tr>`, ""), 1)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "2027-01-23", *rows[1].Date)
}

func TestLegacyHeadingMarkup(t *testing.T) {
	content := strings.ReplaceAll(listHTML("", ""), "<h2>Scheduled events</h2>", `<h2><span id="Scheduled_events">Scheduled events</span><span class="mw-editsection">[edit]</span></h2>`)
	_, err := parseList(content, 1)
	require.ErrorContains(t, err, "incomplete event discovery")
	content = strings.Replace(content, "<th>Date</th><th>Event</th></tr>", `<th>Date</th><th>Event</th></tr><tr><td>TBA</td><td><a href="/wiki/UFC_337">UFC 337</a></td></tr>`, 1)
	rows, err := parseList(content, 1)
	require.NoError(t, err)
	require.Len(t, rows, 1)
}

func TestScopedEnrichment(t *testing.T) {
	date := "2020-05-09"
	f := Facts{Name: "UFC 249: Ferguson vs. Gaethje", Date: &date, Type: "numbered"}
	addName(&f, f.Name, true)
	err := enrich(&f, `<table><tr><td><b>UFC 999</b></td></tr></table><p><b>UFC 249: Ferguson vs. Gaethje</b> was a mixed martial arts event that took place on May 9, 2020. It was originally planned for April 18, 2020.</p><h2>Background</h2><p>At UFC 196 another fight took place.</p>`)
	require.NoError(t, err)
	require.Empty(t, f.Review)
	require.Equal(t, "completed", f.Status)
	for _, a := range f.Aliases {
		require.NotEqual(t, "196", a.Value)
		require.NotEqual(t, "999", a.Value)
	}
	f = Facts{Name: "UFC Fight Night: Hendricks vs. Thompson", Type: "fight_night"}
	addName(&f, f.Name, true)
	require.NoError(t, enrich(&f, `<p><b>UFC Fight Night: Hendricks vs. Thompson</b> (also known as <b>UFC Fight Night 82</b>, originally billed as <b>UFC 196</b>) was a mixed martial arts event held on February 6, 2016.</p>`))
	for _, a := range f.Aliases {
		if a.Namespace == "ufc.numbered" {
			require.False(t, a.Current)
			require.False(t, a.Verified)
		}
	}
	f = Facts{Name: "UFC Fight Night 298", Type: "fight_night"}
	addName(&f, f.Name, true)
	require.NoError(t, enrich(&f, `<p><b>UFC Fight Night 297</b> was a mixed martial arts event.</p>`))
	require.Contains(t, f.Review, "competing_ufc.fight_night")
	for _, a := range f.Aliases {
		if strings.HasPrefix(a.Namespace, "ufc.") {
			require.False(t, a.Verified)
		}
	}
}

func mockWikipedia(t *testing.T, handler http.HandlerFunc) *Wikipedia {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	w := NewWikipedia()
	w.client = &http.Client{Transport: rewriteTransport{u, http.DefaultTransport}, Timeout: time.Second}
	w.backoff = time.Millisecond
	return w
}

func TestHTTPBudgetsAndFailures(t *testing.T) {
	for _, code := range []int{429, 500} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			calls := 0
			w := mockWikipedia(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls < 3 {
					w.WriteHeader(code)
					return
				}
				_, err := w.Write([]byte(`{"query":{"pages":[]}}`))
				require.NoError(t, err)
			})
			_, err := w.request(t.Context(), url.Values{"action": {"query"}})
			require.NoError(t, err)
			require.Equal(t, 3, calls)
		})
	}
	for _, body := range []string{`{"parse":`, `{"error":{"code":"badrevid"}}`, `{"continue":{"x":1}}`, strings.Repeat("x", responseLimit+1)} {
		w := mockWikipedia(t, func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte(body))
			if err != nil && !strings.Contains(err.Error(), "closed") {
				t.Log(err)
			}
		})
		_, err := w.request(t.Context(), url.Values{})
		require.Error(t, err)
	}
	w := mockWikipedia(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() })
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	_, err := w.request(ctx, url.Values{})
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestRetryAfterDeferralBudgets(t *testing.T) {
	for _, delay := range []string{"3600", time.Now().Add(time.Hour).UTC().Format(http.TimeFormat), "1"} {
		var calls atomic.Int32
		w := mockWikipedia(t, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Header().Set("Retry-After", delay)
			w.WriteHeader(429)
		})
		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
		_, err := w.request(ctx, url.Values{})
		cancel()
		require.Error(t, err)
		require.EqualValues(t, 1, calls.Load(), "respect deferral instead of retrying immediately")
	}
}

func TestRedirectResolution(t *testing.T) {
	w := mockWikipedia(t, func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"query": map[string]any{"redirects": []apiRedirect{{From: "UFC old", To: "UFC 300"}, {From: "UFC section", To: "List of UFC events", Fragment: "2027"}, {From: "UFC list", To: "List of UFC events"}}, "pages": []apiPage{{PageID: 1, Title: "UFC 300", Revisions: []struct {
			RevID int64 `json:"revid"`
		}{{2}}}, {PageID: 3, Title: "List of UFC events", Revisions: []struct {
			RevID int64 `json:"revid"`
		}{{4}}}}}}))
	})
	pages, problems, err := w.resolve(t.Context(), []string{"UFC old", "UFC section", "UFC list", "UFC missing"})
	require.NoError(t, err)
	require.EqualValues(t, 1, pages["UFC old"].PageID)
	require.Len(t, problems, 3)
}

func TestAdditionalNumberNamespaces(t *testing.T) {
	for _, tc := range []struct{ name, namespace string }{{"UFC on ABC 9: First vs. Second", "ufc.abc"}, {"UFC Freedom 9: First vs. Second", "ufc.freedom"}} {
		facts := Facts{Type: "fight_night"}
		addName(&facts, tc.name, true)
		found := false
		for _, alias := range facts.Aliases {
			if alias.Namespace == tc.namespace && alias.Value == "9" {
				found = true
				require.True(t, alias.Verified)
				require.NoError(t, ValidateAlias(alias))
			}
		}
		require.True(t, found)
		require.Equal(t, "fight_night", facts.Type)
	}
}

func TestNocheBillingDoesNotInventANumberedEvent(t *testing.T) {
	for _, billing := range []string{"Noche UFC 2", "Riyadh Season Noche UFC 2"} {
		facts := Facts{Name: "UFC 306", Type: "numbered"}
		addName(&facts, facts.Name, true)
		require.NoError(t, enrich(&facts, `<p><b>UFC 306</b> (also known as <b>`+billing+`</b>) was a mixed martial arts event.</p>`))
		require.Empty(t, facts.Review)
		for _, alias := range facts.Aliases {
			if alias.Namespace == "ufc.numbered" {
				require.Equal(t, "306", alias.Value)
			}
		}
	}
}
