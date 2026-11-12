// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRoutineWindowOrderingAndLimit(t *testing.T) {
	row := func(name, date, section, status string) listRow {
		var d *string
		if date != "" {
			d = &date
		}
		return listRow{Facts: Facts{Name: name, Date: d, Section: section, Status: status}}
	}
	rows := []listRow{row("Undated", "", "scheduled events", "scheduled"), row("Later", "2027-01-02", "scheduled events", "scheduled"), row("Sooner", "2026-11-15", "scheduled events", "scheduled"), row("Cancelled", "2026-11-03", "past events", "cancelled"), row("Postponed", "2026-11-02", "past events", "postponed"), row("Unknown date", "", "past events", "unknown")}
	for i := 1; i <= 22; i++ {
		rows = append(rows, row(fmt.Sprint(i), fmt.Sprintf("2026-10-%02d", i), "past events", "unknown"))
	}
	selected := routineRows(rows)
	require.Len(t, selected, 13)
	require.Equal(t, "Sooner", selected[0].Name)
	require.Equal(t, "Later", selected[1].Name)
	require.Equal(t, "Undated", selected[2].Name)
	for i, r := range selected[3:] {
		require.Equal(t, fmt.Sprint(22-i), r.Name)
		require.Equal(t, "unknown", r.Status, "past placement must not imply completion")
	}
}

func TestCompleteHistoryKeepsRoutineEnrichmentBounded(t *testing.T) {
	source, _ := fixtureSource(t)
	snapshot, err := source.ReadScope(t.Context(), []Facts{{PageID: 4294827, Title: "UFC 1"}}, "rebuild")
	require.NoError(t, err)
	require.Greater(t, snapshot.Discovered, 18)
	require.Len(t, snapshot.ListRows, snapshot.Discovered)
	upcoming, completed := 0, 0
	foundFirst, found303 := false, false
	for _, f := range snapshot.Facts {
		if f.Title == "UFC 1" {
			foundFirst = true
			require.NotZero(t, f.ArticleRevision)
		}
		if f.Title == "UFC 303" {
			found303 = true
			require.Equal(t, "696", f.OverallSequence)
		}
		if f.Section == "scheduled events" {
			upcoming++
		} else {
			if f.Status == "completed" {
				completed++
			}
		}
	}
	require.Equal(t, 0, upcoming)
	require.Greater(t, completed, 10)
	require.True(t, foundFirst)
	require.True(t, found303)
}
