// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

func scopedRows(rows []listRow, known []Facts, scope string) []listRow {
	previous := map[string]Facts{}
	transitionDates := map[string]bool{}
	for _, f := range known {
		if f.Section == "scheduled events" && f.Date != nil {
			transitionDates[*f.Date] = true
		}
		previous[f.Title] = f
		for _, a := range f.Aliases {
			if a.Namespace == "title" {
				previous[a.Value] = f
			}
		}
		if f.LinkedTitle != "" {
			previous[f.LinkedTitle] = f
		}
	}
	selected := []listRow{}
	for _, row := range rows {
		old, exists := previous[row.Title]
		if scope == "upcoming" {
			if row.Section == "scheduled events" || row.Section == "past events" && (exists && old.Section == "scheduled events" || row.Date != nil && transitionDates[*row.Date]) {
				selected = append(selected, row)
			}
		} else if row.Section == "past events" && (scope == "rebuild" || !exists || old.Section != "past events" || old.ArticleRevision == 0 || !sameBase(old, row.Facts) || old.OverallSequence != row.OverallSequence) {
			selected = append(selected, row)
		}
	}
	return selected
}
