// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"
)

type fixtureExpectation struct {
	Index      int               `json:"index"`
	Cohort     string            `json:"cohort"`
	Title      string            `json:"title"`
	Name       string            `json:"name"`
	Date       string            `json:"date"`
	PageID     int64             `json:"page_id"`
	Revision   int64             `json:"revision"`
	Aliases    map[string]string `json:"aliases"`
	Headliners []string          `json:"headliners"`
}

type rewriteTransport struct {
	target    *url.URL
	transport http.RoundTripper
}

func (r rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	copy := req.Clone(req.Context())
	u := *req.URL
	u.Scheme = r.target.Scheme
	u.Host = r.target.Host
	copy.URL = &u
	return r.transport.RoundTrip(copy)
}

func fixtureSource(t *testing.T) (*Wikipedia, []fixtureExpectation) {
	t.Helper()
	data, err := os.ReadFile("testdata/wikipedia/expectations.json")
	require.NoError(t, err)
	var expected []fixtureExpectation
	require.NoError(t, json.Unmarshal(data, &expected))
	articles := map[int64][]byte{}
	pages := map[string]apiPage{}
	metadataBytes, err := os.ReadFile("testdata/wikipedia/metadata.json")
	require.NoError(t, err)
	var metadata apiResponse
	require.NoError(t, json.Unmarshal(metadataBytes, &metadata))
	for _, page := range metadata.Query.Pages {
		pages[page.Title] = page
	}
	revisions := map[int64]int64{}
	for _, e := range expected {
		data, err := os.ReadFile(filepath.Join("testdata/wikipedia", "article-"+strconv.Itoa(e.Index)+".json"))
		require.NoError(t, err)
		articles[e.Revision] = data
		revisions[e.PageID] = e.Revision
	}
	files, err := filepath.Glob("testdata/wikipedia/edge-*.json")
	require.NoError(t, err)
	for _, file := range files {
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		var response apiResponse
		require.NoError(t, json.Unmarshal(data, &response))
		p := response.Parse
		articles[p.RevID] = data
		revisions[p.PageID] = p.RevID
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Contains(t, r.UserAgent(), "https://github.com/")
		q := r.URL.Query()
		if q.Get("action") == "parse" {
			if q.Get("oldid") == "1384693275" {
				data, err := os.ReadFile("testdata/wikipedia/list.json")
				require.NoError(t, err)
				_, err = w.Write(data)
				require.NoError(t, err)
				return
			}
			rev, err := strconv.ParseInt(q.Get("oldid"), 10, 64)
			require.NoError(t, err)
			data, ok := articles[rev]
			if !ok {
				http.Error(w, "no article fixture", 404)
				return
			}
			_, err = w.Write(data)
			require.NoError(t, err)
			return
		}
		var result []apiPage
		for _, title := range strings.Split(q.Get("titles"), "|") {
			p, ok := pages[title]
			if !ok {
				p = apiPage{Title: title, Missing: true}
			}
			result = append(result, p)
		}
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"query": map[string]any{"pages": result}}))
	}))
	t.Cleanup(server.Close)
	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	source := NewWikipedia()
	source.client = &http.Client{Transport: rewriteTransport{u, http.DefaultTransport}}
	source.listRevision = 1384693275
	source.articleRevisions = revisions
	source.enrichmentLimit = 64
	return source, expected
}

func TestPinnedSourceGate(t *testing.T) {
	source, expected := fixtureSource(t)
	data, err := os.ReadFile("testdata/wikipedia/list.json")
	require.NoError(t, err)
	var capture apiResponse
	require.NoError(t, json.Unmarshal(data, &capture))
	rows, err := parseList(capture.Parse.Text, capture.Parse.RevID)
	require.NoError(t, err)
	snapshot, err := source.readRows(t.Context(), nil, rows, capture.Parse.RevID)
	require.NoError(t, err)
	type result struct {
		Cohort             string   `json:"cohort"`
		Name               string   `json:"name"`
		Found              bool     `json:"found"`
		DateAgreement      bool     `json:"date_source_agreement"`
		UsableLink         bool     `json:"usable_link"`
		AliasesExpected    int      `json:"aliases_expected"`
		AliasesRecovered   int      `json:"aliases_recovered"`
		AliasGaps          []string `json:"alias_gaps"`
		Review             []string `json:"review"`
		HeadlinersExpected bool     `json:"headliners_expected"`
		HeadlinerAgreement bool     `json:"headliner_source_agreement"`
	}
	var report []result
	for _, e := range expected {
		r := result{Cohort: e.Cohort, Name: e.Name, AliasesExpected: len(e.Aliases), AliasGaps: []string{}, Review: []string{}, HeadlinersExpected: len(e.Headliners) == 2}
		for _, f := range snapshot.Facts {
			if f.PageID != e.PageID {
				continue
			}
			r.Found = true
			r.UsableLink = f.PageID > 0 && f.ArticleRevision == e.Revision
			r.DateAgreement = f.Date != nil && *f.Date == e.Date
			r.Review = f.Review
			if len(f.Headliners) == 2 && len(e.Headliners) == 2 {
				r.HeadlinerAgreement = strings.Contains(referenceFold(f.Headliners[0]), referenceFold(e.Headliners[0])) && strings.Contains(referenceFold(f.Headliners[1]), referenceFold(e.Headliners[1]))
			}
			for ns, v := range e.Aliases {
				found := false
				for _, a := range f.Aliases {
					if a.Namespace == "ufc."+ns && a.Value == v && a.Current && a.Verified {
						found = true
					}
				}
				if found {
					r.AliasesRecovered++
				} else {
					r.AliasGaps = append(r.AliasGaps, ns+":"+v)
				}
			}
		}
		require.True(t, r.Found, "%s", e.Title)
		require.True(t, r.DateAgreement, "%s", e.Title)
		require.True(t, r.UsableLink, "%s", e.Title)
		report = append(report, r)
	}
	for _, f := range snapshot.Facts {
		if strings.Contains(f.Name, "Hendricks") {
			require.Contains(t, f.Aliases, Alias{Namespace: "ufc.numbered", Value: "196", Current: false, Verified: false})
		}
		for _, a := range f.Aliases {
			if a.Current && a.Verified && a.Namespace == "ufc.numbered" {
				require.NotEqual(t, "176", a.Value)
			}
			if a.Current && a.Verified && a.Namespace == "ufc.numbered" && a.Value == "196" {
				require.NotContains(t, f.Name, "Hendricks")
			}
			if a.Current && a.Verified && a.Namespace == "ufc.fight_night" && a.Value == "297" {
				require.NotContains(t, f.Name, "Salkilld")
			}
		}
	}
	data, err = json.MarshalIndent(struct {
		Comparison   string       `json:"comparison"`
		ListRevision int64        `json:"list_revision"`
		Results      []result     `json:"results"`
		Diagnostics  []Diagnostic `json:"diagnostics"`
	}{"source agreement, not independent truth", snapshot.ListRevision, report, snapshot.Diagnostics}, "", "  ")
	require.NoError(t, err)
	t.Log(string(data))
	if out := os.Getenv("UFC_SOURCE_REPORT"); out != "" {
		require.NoError(t, os.WriteFile(out, append(data, '\n'), 0600))
	}
}

func referenceFold(v string) string {
	return strings.Join(strings.Fields(strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Mn, r) {
			return -1
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, norm.NFD.String(v))), " ")
}
