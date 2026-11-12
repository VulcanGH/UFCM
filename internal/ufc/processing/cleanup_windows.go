// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package processing

import (
	"os"

	"golang.org/x/sys/windows"
)

func cleanupLinked(path string, info os.FileInfo) bool {
	if info == nil || info.Mode()&os.ModeSymlink != 0 {
		return true
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return true
	}
	attributes, err := windows.GetFileAttributes(name)
	return err != nil || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0
}

func cleanupWritable(path string, info os.FileInfo) error {
	return os.Chmod(path, info.Mode().Perm()|0200)
}
