// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package updater

import (
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/stretchr/testify/require"
)

func TestDisjointNormalizesWindowsShortPathsAndMissingChildren(t *testing.T) {
	root := t.TempDir()
	long, err := windows.UTF16PtrFromString(root)
	require.NoError(t, err)
	buffer := make([]uint16, 32768)
	n, err := windows.GetShortPathName(long, &buffer[0], uint32(len(buffer)))
	require.NoError(t, err)
	require.Less(t, int(n), len(buffer))
	short := windows.UTF16ToString(buffer[:n])
	require.Error(t, Disjoint(short, root))
	require.Error(t, Disjoint(root, short))
	require.Error(t, Disjoint(filepath.Join(short, "missing", "cache"), root))
	require.Error(t, Disjoint(root, filepath.Join(short, "missing", "library")))
	require.NoError(t, Disjoint(filepath.Join(short, "cache"), filepath.Join(root, "library")))
}
