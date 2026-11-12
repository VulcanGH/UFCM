// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type aliasRequests struct {
	base      http.RoundTripper
	revisions map[string]int
}

func (a *aliasRequests) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Query().Get("action") == "parse" {
		a.revisions[r.URL.Query().Get("oldid")]++
	}
	return a.base.RoundTrip(r)
}

func TestRefreshCachesHistoricalAliases(t *testing.T) {
	source, _ := fixtureSource(t)
	source.enrichmentLimit = 1
	requests := &aliasRequests{base: source.client.Transport, revisions: map[string]int{}}
	source.client.Transport = requests
	snapshot, err := source.ReadScope(t.Context(), nil, "past")
	require.NoError(t, err)
	var historical Facts
	for _, f := range snapshot.Facts {
		if f.ArticleRevision == 0 && strings.HasPrefix(f.Title, "UFC Fight Night:") && f.articleTarget > 0 {
			historical = f
			break
		}
	}
	require.NotZero(t, historical.PageID)
	checkpoints := 0
	err = source.EnrichMissing(t.Context(), snapshot, func(_ context.Context, patch *Snapshot) error {
		checkpoints++
		require.LessOrEqual(t, len(patch.Facts), 16)
		require.Greater(t, patch.AliasProcessed, 0)
		return nil
	})
	require.NoError(t, err)
	require.Greater(t, checkpoints, 0)
	found := false
	for _, f := range snapshot.Facts {
		if f.PageID == historical.PageID {
			require.NotZero(t, f.ArticleRevision)
			for _, a := range f.Aliases {
				if a.Namespace == "ufc.fight_night" && a.Current && a.Verified {
					found = true
				}
			}
		}
	}
	require.True(t, found, "historical Fight Night number must be populated")
	revision := strconv.FormatInt(historical.articleTarget, 10)
	before := requests.revisions[revision]
	require.Equal(t, 1, before)
	raw, err := json.Marshal(snapshot.Facts)
	require.NoError(t, err)
	var known []Facts
	require.NoError(t, json.Unmarshal(raw, &known))
	warm, err := source.ReadScope(t.Context(), known, "past")
	require.NoError(t, err)
	require.NoError(t, source.EnrichMissing(t.Context(), warm, func(context.Context, *Snapshot) error { return nil }))
	require.Equal(t, before, requests.revisions[revision], "unchanged historical aliases must come from saved facts")
}
