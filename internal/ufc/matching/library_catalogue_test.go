// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package matching

import (
	"fmt"
	"testing"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"

	"github.com/stretchr/testify/require"
)

func numberedSourceEvent(number int) catalogue.Event {
	date := "2009-07-11"
	name := fmt.Sprintf("UFC %d", number)
	return catalogue.Event{ID: name, Values: catalogue.Values{Name: name, Type: "numbered", Date: &date}, Source: catalogue.Facts{PageID: int64(number), Title: name, Type: "numbered"}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: fmt.Sprint(number), Current: true}}}
}

func TestLibraryCanonicalNumberIsIndependentOfBillingAndVerificationFlags(t *testing.T) {
	for number := 1; number <= 400; number++ {
		event := numberedSourceEvent(number)
		event.Aliases[0].Current = number%2 == 0
		for _, title := range []string{fmt.Sprintf("UFC %d Alternate Billing II", number), fmt.Sprintf("UFC.%d.New-Names-vs-Other-Names.2009", number)} {
			p := MatchLibrary(title, []catalogue.Event{event})
			require.Equal(t, "matched", p.Status, title)
			require.Equal(t, event.ID, p.Candidates[0].ID)
			require.NotEqual(t, "matched", Match(title, []catalogue.Event{event}).Status, "acquisition verification remains strict")
		}
	}
	event := numberedSourceEvent(100)
	event.Aliases = nil
	require.Equal(t, "matched", MatchLibrary("UFC 100", []catalogue.Event{event}).Status)
	event.Corrections.Aliases = &catalogue.AliasCorrection{Protected: true}
	require.NotEqual(t, "matched", MatchLibrary("UFC 100", []catalogue.Event{event}).Status)
}

func TestLibraryQuarantinesStaleCrossFamilyAliasesWithoutChangingCatalogue(t *testing.T) {
	one := numberedSourceEvent(2)
	one.Review = []string{"identifier_owned_by_existing_event"}
	other := numberedSourceEvent(306)
	other.Review = []string{"competing_ufc.numbered", "identifier_owned_by_existing_event"}
	other.Aliases = append(other.Aliases, catalogue.Alias{Namespace: "ufc.numbered", Value: "2", Current: true})
	events := []catalogue.Event{one, other}
	for _, event := range events {
		p := MatchLibrary(event.Values.Name, events)
		require.Equal(t, "matched", p.Status)
		require.Equal(t, event.ID, p.Candidates[0].ID)
	}
	require.Len(t, events[1].Aliases, 2)
	require.Len(t, events[1].Review, 2)
	duplicate := one
	duplicate.ID = "genuine-second-owner"
	require.Equal(t, "ambiguous", MatchLibrary("UFC 2", append(events, duplicate)).Status)
}

func TestLibraryNumberedIdentitySeparatesDateQualityFromIdentityConflicts(t *testing.T) {
	event := numberedSourceEvent(100)
	event.Review = []string{"article_list_date_conflict", "article_list_headliner_conflict"}
	require.Equal(t, "matched", MatchLibrary("UFC 100 Other Billing (2009)", []catalogue.Event{event}).Status)
	require.Equal(t, "needs review", MatchLibrary("UFC 100 Other Billing (2010)", []catalogue.Event{event}).Status)
	event.Review = []string{"article_identity_conflict"}
	require.Equal(t, "needs review", MatchLibrary("UFC 100", []catalogue.Event{event}).Status)
	event = numberedSourceEvent(100)
	require.NotEqual(t, "matched", MatchLibrary("UFC Fight Night 100", []catalogue.Event{event}).Status)
	require.NotEqual(t, "matched", MatchLibrary("UFC on ESPN 100", []catalogue.Event{event}).Status)
	require.NotEqual(t, "matched", MatchLibrary("Noche UFC 100", []catalogue.Event{event}).Status)
	require.Equal(t, []Identifier{{Namespace: "ufc.numbered", Value: "100"}}, identifiers(folded("Noche UFC 100 and UFC 100")))
}

func TestMissingNumberedIdentityDoesNotFallThroughToRematch(t *testing.T) {
	event := numberedSourceEvent(169)
	event.Values.Name = "UFC 169: Barão vs. Faber 2"
	event.Values.Headliners = []string{"Barão", "Faber"}
	p := MatchLibrary("UFC 149 Faber vs. Barao (2012)", []catalogue.Event{event})
	require.Equal(t, "unmatched", p.Status)
	require.Empty(t, p.Candidates)
}

func TestLibraryKeepsFightNightAndPlainNumberedIdentitiesSeparate(t *testing.T) {
	one := numberedSourceEvent(3)
	date := "2009-07-11"
	night := catalogue.Event{ID: "night", Values: catalogue.Values{Name: "UFC Fight Night: Alpha vs. Beta", Type: "fight_night", Date: &date}, Source: catalogue.Facts{PageID: 500, Title: "UFC Fight Night: Alpha vs. Beta", Type: "fight_night"}, Aliases: []catalogue.Alias{{Namespace: "ufc.numbered", Value: "3", Current: true}, {Namespace: "ufc.fight_night", Value: "321", Current: true}}, Review: []string{"article_list_type_conflict", "identifier_owned_by_existing_event"}}
	events := []catalogue.Event{one, night}
	plain := MatchLibrary("UFC 3", events)
	require.Equal(t, "matched", plain.Status)
	require.Equal(t, one.ID, plain.Candidates[0].ID)
	labelled := MatchLibrary("UFC Fight Night 321 Alpha vs. Beta (2009)", events)
	require.Equal(t, "matched", labelled.Status)
	require.Equal(t, night.ID, labelled.Candidates[0].ID)
}
