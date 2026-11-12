// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package processing

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestCleanupRetriesWindowsLockAndReadOnlyFile(t *testing.T) {
	service, job, storage, libraryRoot := cleanupFixture(t, "UFCM-owned", "card.mkv")
	path := filepath.Join(storage, "card.mkv")
	require.NoError(t, os.Chmod(path, 0400))
	name, err := windows.UTF16PtrFromString(path)
	require.NoError(t, err)
	handle, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		time.Sleep(200 * time.Millisecond)
		done <- windows.CloseHandle(handle)
	}()
	require.Empty(t, service.cleanupCompleted(t.Context(), job))
	require.NoError(t, <-done)
	require.NoDirExists(t, storage)
	require.FileExists(t, filepath.Join(libraryRoot, "event.mkv"))
}
