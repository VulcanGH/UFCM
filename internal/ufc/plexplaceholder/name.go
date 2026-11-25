// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package plexplaceholder reserves a Plex-facing filename that is not UFC media.
package plexplaceholder

import "strings"

const Suffix = " - UFCM Placeholder.mkv"

func IsName(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), strings.ToLower(Suffix))
}
