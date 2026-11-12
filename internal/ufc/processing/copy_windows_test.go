// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package processing

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"

	"github.com/stretchr/testify/require"
)

func TestCopyFromWindowsShortPathPreservesMedia(t *testing.T) {
	root := filepath.Join(t.TempDir(), "completed download fixtures")
	require.NoError(t, os.Mkdir(root, 0700))
	path, err := windows.UTF16PtrFromString(root)
	require.NoError(t, err)
	buffer := make([]uint16, 32768)
	n, err := windows.GetShortPathName(path, &buffer[0], uint32(len(buffer)))
	require.NoError(t, err)
	require.Less(t, int(n), len(buffer))
	short := windows.UTF16ToString(buffer[:n])
	resolved, err := filepath.EvalSymlinks(short)
	require.NoError(t, err)
	if strings.EqualFold(short, resolved) {
		t.Skip("fixture volume does not expose Windows short-path aliases")
	}
	t.Setenv("TEMP", short)
	t.Setenv("TMP", short)
	t.Run("copy", TestCopyPreservesSourceAndNeverReplacesExistingMedia)
	t.Run("keep both", TestKeepBothRequiresReviewAndPreservesConflict)
	t.Run("bundle", TestMatchedMultiFileBundleGetsDistinctNames)
}
