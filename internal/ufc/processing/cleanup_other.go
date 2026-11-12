// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

//go:build !windows

package processing

import "os"

func cleanupLinked(_ string, info os.FileInfo) bool {
	return info == nil || info.Mode()&os.ModeSymlink != 0
}

func cleanupWritable(_ string, _ os.FileInfo) error {
	return os.ErrPermission
}
