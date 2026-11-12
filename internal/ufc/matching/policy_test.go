// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package matching

import (
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestConfiguredTermsAndNamespaces(t *testing.T) {
	events := []catalogue.Event{{ID: "abc", Values: catalogue.Values{Name: "UFC on ABC 9", Status: "completed"}, Aliases: []catalogue.Alias{{Namespace: "ufc.abc", Value: "9", Current: true, Verified: true}}}, {ID: "freedom", Values: catalogue.Values{Name: "UFC Freedom 9", Status: "completed"}, Aliases: []catalogue.Alias{{Namespace: "ufc.freedom", Value: "9", Current: true, Verified: true}}}}
	p := MatchWithTerms("UFC.on.ABC.9.Special.Show.1080p", events, []string{"special show"})
	require.Equal(t, "matched", p.Status)
	require.Equal(t, "abc", p.Candidates[0].ID)
	require.Equal(t, "excluded", p.MainCard.Status)
	require.Empty(t, p.Components.Values)
	p = MatchWithTerms("UFC.on.ABC.9.Pre-Show.1080p", events, []string{})
	require.Equal(t, []string{"main_card"}, p.Components.Values)
	require.Equal(t, "excluded", Match("UFC.on.ABC.9.Pre-Show.1080p", events).MainCard.Status)
	p = Match("UFC.Freedom.9.1080p", events)
	require.Equal(t, "matched", p.Status)
	require.Equal(t, "freedom", p.Candidates[0].ID)
	require.Equal(t, "ambiguous", Match("UFC.on.ABC.9.UFC.Freedom.9.1080p", events).Status)
	require.NotEqual(t, "matched", Match("UFC.on.ABC.10.1080p", events).Status)
}
