// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package catalogue

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProtectedEditsAndExplicitSourceReset(t *testing.T) {
	date, name, status := "2027-01-30", "My event", "postponed"
	f := Facts{PageID: 1, ListRevision: 1, RetrievedAt: time.Now(), Title: "UFC 336", Name: "UFC 336", Type: "numbered", Date: &date, Headliners: []string{"Van", "Kape"}, Status: "scheduled"}
	e, err := Merge(Event{ID: "local"}, f)
	require.NoError(t, err)
	e.Revision = 1
	edited, err := ApplyEdit(e, EventEdit{Revision: 1, Name: &FieldEdit{Operation: "set", Value: &name}, Date: &FieldEdit{Operation: "set"}, Headliners: &FieldEdit{Operation: "set"}, Status: &FieldEdit{Operation: "set", Value: &status}}, "user:test", time.Now())
	require.NoError(t, err)
	require.Equal(t, "user:test", edited.Corrections.Name.Actor)
	require.True(t, edited.Corrections.Date.Protected)
	require.Nil(t, edited.Values.Date)
	require.Empty(t, edited.Values.Headliners)
	f.Name = "Wikipedia new billing"
	f.Status = "completed"
	refreshed, err := Merge(edited, f)
	require.NoError(t, err)
	require.Equal(t, name, refreshed.Values.Name)
	require.Equal(t, status, refreshed.Values.Status)
	require.Nil(t, refreshed.Values.Date)
	require.Empty(t, refreshed.Values.Headliners)
	reset, err := ApplyEdit(refreshed, EventEdit{Revision: 1, Name: &FieldEdit{Operation: "reset"}, Date: &FieldEdit{Operation: "reset"}, Headliners: &FieldEdit{Operation: "reset"}, Status: &FieldEdit{Operation: "reset"}}, "user:test", time.Now())
	require.NoError(t, err)
	require.Equal(t, f.Name, reset.Values.Name)
	require.Equal(t, f.Status, reset.Values.Status)
	require.Equal(t, f.Date, reset.Values.Date)
	require.Equal(t, f.Headliners, reset.Values.Headliners)
	require.Equal(t, Corrections{}, reset.Corrections)
	// An explicit reset accepts unknown source values instead of retaining a manual value.
	refreshed.Source.Date = nil
	refreshed.Source.Headliners = nil
	refreshed.Source.Status = "unknown"
	reset, err = ApplyEdit(refreshed, EventEdit{Revision: 1, Date: &FieldEdit{Operation: "reset"}, Headliners: &FieldEdit{Operation: "reset"}, Status: &FieldEdit{Operation: "reset"}}, "user:test", time.Now())
	require.NoError(t, err)
	require.Nil(t, reset.Values.Date)
	require.Empty(t, reset.Values.Headliners)
	require.Equal(t, "unknown", reset.Values.Status)
	require.Nil(t, reset.Corrections.Date)
	require.NotNil(t, reset.Corrections.Name, "unmentioned protection is retained")
	require.Equal(t, "local", reset.ID)
}

func TestEventEditValidationAndRevision(t *testing.T) {
	date := "2027-01-30"
	e := Event{Revision: 2, Values: Values{Name: "Event", Type: "numbered", Date: &date, Status: "scheduled"}}
	bad, status := "", "invalid"
	for _, edit := range []EventEdit{
		{Revision: 2},
		{Revision: 2, Name: &FieldEdit{Operation: "set", Value: &bad}},
		{Revision: 2, Name: &FieldEdit{Operation: "set"}},
		{Revision: 2, Date: &FieldEdit{Operation: "unlock"}},
		{Revision: 2, Date: &FieldEdit{Operation: "set", Value: &bad}},
		{Revision: 2, Date: &FieldEdit{Operation: "reset", Value: &date}},
		{Revision: 2, Headliners: &FieldEdit{Operation: "set", Names: []string{"Only one"}}},
		{Revision: 2, Status: &FieldEdit{Operation: "set", Value: &status}},
	} {
		_, err := ApplyEdit(e, edit, "test", time.Now())
		require.ErrorIs(t, err, ErrInvalidEdit)
	}
	_, err := ApplyEdit(e, EventEdit{Revision: 1, Date: &FieldEdit{Operation: "set"}}, "test", time.Now())
	require.ErrorIs(t, err, ErrRevisionConflict)
}
