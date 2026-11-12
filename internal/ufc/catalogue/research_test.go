// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

//go:build source_research

package catalogue

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPinnedFullCapture(t *testing.T) {
	path := os.Getenv("UFC_FULL_WIKIPEDIA_CAPTURE")
	require.NotEmpty(t, path, "full pinned API capture is an explicit research prerequisite")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var response apiResponse
	require.NoError(t, json.Unmarshal(data, &response))
	require.EqualValues(t, 1384693275, response.Parse.RevID)
	rows, err := parseList(response.Parse.Text, response.Parse.RevID)
	require.NoError(t, err)
	_, expected := fixtureSource(t)
	for _, e := range expected {
		found := false
		for _, row := range rows {
			if row.Title == e.Title {
				require.NotNil(t, row.Date)
				require.Equal(t, e.Date, *row.Date)
				found = true
			}
		}
		require.True(t, found, e.Title)
	}
	t.Logf("full pinned list: %d rows; all 26 manual sample dates agree", len(rows))
}
