// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSourceIdentityConflictsCacheAndEnrichmentFailures(t *testing.T) {
	for _, scenario := range []string{"cache_and_failure", "duplicate_number", "redirect_changes_identity", "duplicate_link"} {
		t.Run(scenario, func(t *testing.T) {
			revision := int64(1)
			listDate := "Jan 23, 2027"
			articleCalls := 0
			failArticle := false
			w := mockWikipedia(t, func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				if q.Get("action") == "query" {
					var p1, p2 apiPage
					p1.PageID = 1
					p1.Title = "UFC Fight Night 297"
					p1.Revisions = append(p1.Revisions, struct {
						RevID int64 `json:"revid"`
					}{revision})
					p2.PageID = 2
					p2.Title = "UFC Fight Night 298"
					p2.Revisions = p1.Revisions
					redirects := []apiRedirect{}
					if scenario == "duplicate_link" || scenario == "redirect_changes_identity" {
						redirects = append(redirects, apiRedirect{From: p2.Title, To: p1.Title})
						p2 = p1
					}
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"query": map[string]any{"pages": []apiPage{p1, p2}, "redirects": redirects}}))
					return
				}
				if q.Get("page") == "List of UFC events" {
					content := listHTML(`<tr><td>`+listDate+`</td><td><a href="/wiki/UFC_Fight_Night_297">UFC Fight Night 297</a></td></tr><tr><td>Feb 7, 2027</td><td><a href="/wiki/UFC_Fight_Night_298">UFC Fight Night 298</a></td></tr>`, "")
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"parse": map[string]any{"pageid": 100, "revid": 100, "title": "List of UFC events", "text": content}}))
					return
				}
				articleCalls++
				if failArticle {
					w.WriteHeader(500)
					return
				}
				// These synthetic pages deliberately share a revision value; the requested page determines the body below.
				id := int64(1)
				title := "UFC Fight Night 297"
				if articleCalls%2 == 0 {
					id = 2
					title = "UFC Fight Night 298"
				}
				if scenario == "duplicate_link" || scenario == "redirect_changes_identity" {
					id = 1
					title = "UFC Fight Night 297"
				}
				name := title
				if scenario == "duplicate_number" && id == 2 {
					name = "UFC Fight Night 297"
				}
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"parse": map[string]any{"pageid": id, "revid": revision, "title": title, "text": "<p><b>" + name + "</b> is an upcoming mixed martial arts event.</p>"}}))
			})
			known := []Facts{}
			if scenario == "redirect_changes_identity" {
				known = []Facts{{PageID: 2, Title: "UFC Fight Night 298"}}
			}
			snapshot, err := w.Read(t.Context(), known)
			require.NoError(t, err)
			switch scenario {
			case "duplicate_link", "redirect_changes_identity":
				require.Len(t, snapshot.Facts, 1)
				require.NotEmpty(t, snapshot.Diagnostics)
			case "duplicate_number":
				require.Len(t, snapshot.Facts, 2)
				require.NotEmpty(t, snapshot.Facts[1].Review)
				for _, f := range snapshot.Facts {
					for _, a := range f.Aliases {
						if a.Namespace == "ufc.fight_night" && a.Value == "297" && f.PageID == 2 {
							require.False(t, a.Verified)
						}
					}
				}
			case "cache_and_failure":
				require.Equal(t, 2, articleCalls)
				again, err := w.Read(t.Context(), snapshot.Facts)
				require.NoError(t, err)
				require.Equal(t, 2, articleCalls)
				require.Len(t, again.Facts, 2)
				listDate = "Jan 24, 2027"
				changed, err := w.Read(t.Context(), again.Facts)
				require.NoError(t, err)
				require.Equal(t, 3, articleCalls, "changed list facts require rechecking the cached article")
				require.Equal(t, "2027-01-24", *changed.Facts[0].Date)
				revision++
				failArticle = true
				failed, err := w.Read(t.Context(), again.Facts)
				require.NoError(t, err)
				require.Len(t, failed.Facts, 2)
				for _, f := range failed.Facts {
					require.Contains(t, f.Review, "article_enrichment_unavailable")
					require.EqualValues(t, 1, f.ArticleRevision)
				}
				require.Equal(t, 9, articleCalls)
			}
		})
	}
}

func TestTruncatedHTMLAndZeroRowspan(t *testing.T) {
	_, err := parseList(strings.TrimSuffix(listHTML("", ""), "</table>"), 1)
	require.Error(t, err)
	rows, err := parseList(listHTML(`<tr><td rowspan="0">2027-01-23</td><td><a href="/wiki/UFC_337">UFC 337</a></td></tr><tr><td><a href="/wiki/UFC_338">UFC 338</a></td></tr>`, ""), 1)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, "2027-01-23", *rows[1].Date)
	for _, f := range rows {
		for _, a := range f.Aliases {
			if a.Namespace == "ufc.numbered" {
				_, err := strconv.Atoi(a.Value)
				require.NoError(t, err)
			}
		}
	}
}
