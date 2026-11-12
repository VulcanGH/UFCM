package catalogue

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRefreshScopes(t *testing.T) {
	row := func(title, section string) listRow {
		return listRow{Facts: Facts{Title: title, Name: title, Type: "fight_night", Section: section}}
	}
	rows := []listRow{row("Upcoming", "scheduled events"), row("Moved", "past events"), row("Established", "past events"), row("New", "past events"), row("Unfinished", "past events"), row("Changed", "past events")}
	known := []Facts{}
	for _, r := range []listRow{rows[1], rows[2], rows[4], rows[5]} {
		f := r.Facts
		f.ArticleRevision = 1
		known = append(known, f)
	}
	known[0].Section = "scheduled events"
	known[2].ArticleRevision = 0
	known[3].Name = "Earlier name"
	titles := func(scope string) []string {
		result := []string{}
		for _, r := range scopedRows(rows, known, scope) {
			result = append(result, r.Title)
		}
		return result
	}
	require.Equal(t, []string{"Upcoming", "Moved"}, titles("upcoming"))
	require.Equal(t, []string{"Moved", "New", "Unfinished", "Changed"}, titles("past"))
	require.Equal(t, []string{"Moved", "Established", "New", "Unfinished", "Changed"}, titles("rebuild"))
}
