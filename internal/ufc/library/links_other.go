// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

//go:build !windows

package library

import "io/fs"

func reparse(info fs.FileInfo) bool { return info.Mode()&fs.ModeSymlink != 0 }
