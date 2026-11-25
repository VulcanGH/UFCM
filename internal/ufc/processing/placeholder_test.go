// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package processing

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/autobrr/autobrr/internal/ufc/plexplaceholder"
	"github.com/stretchr/testify/require"
)

func TestPlexPlaceholderLifecycle(t *testing.T) {
	root := t.TempDir()
	const title = "UFC 999 (2026)"
	event := filepath.Join(root, title)
	require.NoError(t, os.MkdirAll(filepath.Join(event, "Featurettes"), 0755))
	write := func(relative string, content []byte) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(root, relative), content, 0644))
	}
	extras := PlaceholderTask{Root: root, Destination: title + "/Featurettes/Prelims.mkv", Component: "prelims", Size: 5}
	main := PlaceholderTask{Root: root, Destination: title + "/" + title + ".mp4", Component: "main_card", Size: 5}
	write(extras.Destination, []byte("extra"))
	write(title+"/poster.jpg", []byte("artwork"))
	name := filepath.Join(event, title+plexplaceholder.Suffix)
	settings := &Settings{}
	require.NoError(t, reconcilePlaceholder(extras, settings))
	require.NoFileExists(t, name)

	settings.PlexPlaceholder = true
	require.NoError(t, reconcilePlaceholder(extras, settings))
	require.NoError(t, reconcilePlaceholder(extras, settings))
	data, err := os.ReadFile(name)
	require.NoError(t, err)
	require.Equal(t, placeholderVideo, data)

	require.Error(t, reconcilePlaceholder(main, settings))
	require.FileExists(t, name)
	write(main.Destination, []byte("movie"))
	settings.PlexPlaceholder = false
	require.NoError(t, reconcilePlaceholder(main, settings))
	require.NoFileExists(t, name)
	require.NoFileExists(t, filepath.Join(event, placeholderOwner))
	require.FileExists(t, filepath.Join(root, main.Destination))

	settings.PlexPlaceholder = true
	require.NoError(t, reconcilePlaceholder(extras, settings))
	require.NoFileExists(t, name)
	require.NoError(t, os.Remove(filepath.Join(root, main.Destination)))
	require.NoError(t, reconcilePlaceholder(extras, settings))
	require.NoError(t, os.Remove(name))
	write(title+"/"+title+plexplaceholder.Suffix, placeholderVideo)
	write(main.Destination, []byte("movie"))
	require.Error(t, reconcilePlaceholder(main, settings))
	require.FileExists(t, name)
}
