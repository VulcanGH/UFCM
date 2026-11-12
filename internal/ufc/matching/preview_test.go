// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package matching

import (
	"testing"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/stretchr/testify/require"
)

func TestPreview(t *testing.T) {
	date := "2026-11-07"
	events := []catalogue.Event{
		{ID: "numbered", Values: catalogue.Values{Name: "UFC 300", Date: &date, Status: "completed", Headliners: []string{"Alex Pereira", "Jamahal Hill"}}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "300", Current: true, Verified: true}}},
		{ID: "night", Values: catalogue.Values{Name: "UFC Fight Night: Other vs. Fighters", Status: "scheduled"}, Aliases: []catalogue.Alias{{Namespace: "ufc.fight_night", Value: "300", Current: true, Verified: true}, {Namespace: "ufc.vegas", Value: "99", Current: true, Verified: true}, {Namespace: "ufc.espn_plus", Value: "50", Current: true, Verified: true}}},
	}
	for _, tc := range []struct{ title, status, id string }{
		{"UFC.300.Main.Card.1080p.WEBRip.x264", "matched", "numbered"},
		{"UFC_FN_300_Prelims_720p", "matched", "night"},
		{"UFC.Vegas.99.Early.Prelims", "matched", "night"},
		{"UFC.on.ESPN+.50.Main.Card", "matched", "night"},
		{"UFC.on.ESPN.50.Main.Card", "unmatched", ""},
		{"300.Main.Card.1080p", "unmatched", ""},
		{"UFC.3000.Main.Card", "unmatched", ""},
		{"UFC.300.UFC.Fight.Night.300", "ambiguous", ""},
		{"UFC.300.2026.11.08", "needs review", "numbered"},
		{"Pereira.vs.Hill.2026.11.07", "matched", "numbered"},
		{"Pereira.vs.Hill", "needs review", "numbered"},
	} {
		t.Run(tc.title, func(t *testing.T) {
			p := Match(tc.title, events)
			require.Equal(t, tc.status, p.Status)
			if tc.id != "" {
				require.Equal(t, tc.id, p.Candidates[0].ID)
			}
		})
	}
	p := Match("UFC.300.Early.Prelims.1080p.WEBRip.x264", events)
	require.Equal(t, []string{"early_prelims"}, p.Components.Values)
	require.Equal(t, "title assertions", p.Quality.Status)
	require.Equal(t, "conflicting resolutions", AssessQuality("1080p.720p").Status)
	require.Equal(t, "unknown", ClassifyComponents("UFC.300.1080p").Status)
	require.ElementsMatch(t, []string{"main_card", "prelims"}, ClassifyComponents("Main.Card.and.Prelims").Values)
	events[0].Corrections.Deleted = true
	require.Equal(t, "unmatched", Match("UFC.300", events).Status)
	events[0].Corrections.Deleted = false
	events[0].Aliases[0].Verified = false
	require.Equal(t, "needs review", Match("UFC.300", events).Status)
	events[0].Aliases[0].Verified = true
	events = append(events, events[0])
	events[2].ID = "rematch"
	require.Equal(t, "ambiguous", Match("Pereira.Hill.2026.11.07", events).Status)
}

func TestMainCardTerms(t *testing.T) {
	for _, title := range []string{
		"UFC.Fight.Night.293.Pre-Show.1080p.HDTV.h264-Star",
		"UFC.300.Main.Card.Press.Conference", "UFC_300_Prelims", "UFC.300.Early.Prelims",
		"UFC.300.Ceremonial.Weigh-Ins", "UFC.300.Weighins", "UFC.300.Faceoff",
		"UFC.300.PostShow", "UFC.300.Fight.Motion", "UFC.300.The.Thrill.&.The.Agony",
		"UFC.300.Free.Fight", "UFC.300.Embedded", "UFC.300.Cold.Open", "UFC.300.Open.Workout",
	} {
		t.Run(title, func(t *testing.T) {
			c := CheckMainCard(title, ClassifyComponents(title))
			require.Equal(t, "excluded", c.Status)
			require.NotEmpty(t, c.MatchedTerms)
		})
	}
	require.Equal(t, "explicit label", CheckMainCard("UFC.300.Main.Card.1080p", ClassifyComponents("Main.Card")).Status)
	require.Equal(t, "inferred label", CheckMainCard("UFC.300.Full.Event.Replay.1080p", ClassifyComponents("UFC.300")).Status)
	require.Empty(t, CheckMainCard("UFC.Pressure.Main.Card", ClassifyComponents("Main.Card")).MatchedTerms)
	p := Match("UFC.300.Main.Card.Pre-Show", []catalogue.Event{{ID: "event", Values: catalogue.Values{Status: "scheduled"}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "300", Current: true, Verified: true}}}})
	require.Equal(t, "matched", p.Status)
	require.Equal(t, "excluded", p.MainCard.Status)
	p = Match("UFC.300.1080p.HDTV", []catalogue.Event{{ID: "event", Values: catalogue.Values{Status: "scheduled"}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "300", Current: true, Verified: true}}}})
	require.Equal(t, []string{"main_card"}, p.Components.Values)
	require.Equal(t, "inferred", p.Components.Status)
	require.Equal(t, "unknown", Match("UFC.999.1080p", nil).Components.Status)
}
