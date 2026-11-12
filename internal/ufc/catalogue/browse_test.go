package catalogue

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBrowsePermanentCatalogue(t *testing.T) {
	events := []Event{}
	for i := 0; i < 105; i++ {
		date := fmt.Sprintf("%04d-01-01", 1900+i)
		events = append(events, Event{ID: fmt.Sprint(i), Values: Values{Name: fmt.Sprintf("UFC %d", i), Date: &date}})
	}
	page := Browse(events, "", 0)
	require.Equal(t, 105, page.Total)
	require.Len(t, page.Events, 50)
	require.Equal(t, "104", page.Events[0].ID)
	require.Len(t, Browse(events, "", 100).Events, 5)
	require.Empty(t, Browse(events, "", 1000).Events)
	events[0].Aliases = []Alias{{Value: "Historical special"}}
	events[0].Source.OverallSequence = "696"
	require.Equal(t, "0", Browse(events, "historical special", 0).Events[0].ID)
	require.Equal(t, "0", Browse(events, "696", 0).Events[0].ID)
	events[0].Corrections.Deleted = true
	require.Empty(t, Browse(events, "historical special", 0).Events)
	for i := range events {
		events[i].Values.Status = "completed"
	}
	events[104].Values.Status = "scheduled"
	require.Equal(t, 103, BrowseWindow(events, "", 0, 20, true, "2026-11-10").Total)
	require.Len(t, BrowseWindow(events, "", 0, 20, true, "2026-11-10").Events, 20)
	require.Len(t, BrowseWindow(events, "", 0, 2500, true, "2026-11-10").Events, 103)
}
