// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package processing

import (
	"path/filepath"
	"testing"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/stretchr/testify/require"
)

func TestSettingsValidationKeepsLibrarySeparate(t *testing.T) {
	base := t.TempDir()
	libraryRoot := filepath.Join(base, "library")
	downloadRoot := filepath.Join(base, "downloads")
	s := Settings{Revision: 1, SourceRoots: []string{downloadRoot}, DestinationRoot: libraryRoot, EventFolderTemplate: "{{.EventTitle}} ({{.Year}})", MainFileTemplate: "{{.EventTitle}} ({{.Year}})", ExtraFileTemplate: "{{.EventTitle}} ({{.Year}}) - {{.Component}}", ComponentFolder: "Featurettes", ConflictPolicy: "review", Extensions: []string{".mkv", ".mp4"}, MaxConcurrent: 1, MaxAttempts: 3, SABClientID: 1, CleanupSABDownload: true}
	require.NoError(t, Validate(&s, []string{libraryRoot}, []string{libraryRoot}))
	s.SourceRoots = []string{filepath.Join(libraryRoot, "incoming")}
	require.ErrorIs(t, Validate(&s, []string{libraryRoot}, []string{libraryRoot}), catalogue.ErrInvalidEdit)
	s.SourceRoots = []string{downloadRoot}
	s.ConflictPolicy = "replace"
	require.ErrorIs(t, Validate(&s, []string{libraryRoot}, []string{libraryRoot}), catalogue.ErrInvalidEdit)
	s.ConflictPolicy = "review"
	s.EventFolderTemplate = "{{.Unknown}}"
	require.ErrorIs(t, Validate(&s, []string{libraryRoot}, []string{libraryRoot}), catalogue.ErrInvalidEdit)
	s.EventFolderTemplate = "{{.EventTitle}} ({{.Year}})"
	s.DestinationRoot = ""
	require.ErrorIs(t, Validate(&s, []string{libraryRoot}, []string{libraryRoot}), catalogue.ErrInvalidEdit)
}
