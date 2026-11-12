// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLocalComponentsAndProtectedUnknowns(t *testing.T) {
	templates := DefaultTemplates()
	require.NoError(t, ValidateTemplates(templates))
	require.Equal(t, []string{"main_card", "prelims", "early_prelims"}, Components("numbered", templates, nil))
	require.Equal(t, []string{"main_card", "prelims"}, Components("fight_night", templates, nil))
	overrides := map[string]bool{"prelims": false, "embedded": true}
	require.Equal(t, []string{"main_card", "early_prelims", "embedded"}, Components("numbered", templates, overrides))
	require.Equal(t, []string{"main_card", "embedded"}, Components("fight_night", templates, overrides))
	require.Error(t, ValidateOverrides(map[string]bool{"unknown": true}))
	date := "2027-01-30"
	f := Facts{PageID: 1, ListRevision: 1, RetrievedAt: time.Now().UTC(), Title: "UFC 336", Name: "UFC 336", Type: "numbered", Date: &date, Status: "scheduled", Headliners: []string{"Van", "Kape"}}
	event, err := Merge(Event{ID: "immutable", Corrections: Corrections{Date: &Correction{Protected: true}, Headliners: &Correction{Protected: true}}}, f)
	require.NoError(t, err)
	require.Nil(t, event.Values.Date)
	require.Empty(t, event.Values.Headliners)
	require.Equal(t, "immutable", event.ID)
	require.Equal(t, date, *event.Source.Date)
	event.Corrections = Corrections{}
	reset, err := Merge(event, event.Source)
	require.NoError(t, err)
	require.Equal(t, date, *reset.Values.Date)
	require.Len(t, reset.Values.Headliners, 2)
	reset.Source.PageID = 2
	_, err = Merge(event, reset.Source)
	require.Error(t, err, "a source redirect cannot replace canonical identity")
}
