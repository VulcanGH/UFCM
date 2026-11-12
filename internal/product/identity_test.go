// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package product

import "testing"

func TestIdentity(t *testing.T) {
	for _, tc := range []struct {
		id                      Identity
		slug, name, prefix, key string
		port                    int
		updates                 bool
	}{
		{Autobrr(), "autobrr", "autobrr", "AUTOBRR__", `Software\autobrr\autobrr-desktop`, 7474, true},
		{UFCManager(), "ufc-manager", "UFC Manager", "UFC_MANAGER__", `Software\ufc-manager\ufc-manager-desktop`, 7475, false},
	} {
		if tc.id.Slug() != tc.slug || tc.id.Name() != tc.name || tc.id.Port() != tc.port || tc.id.EnvPrefix() != tc.prefix || tc.id.RegistryKey() != tc.key || tc.id.UpdatesEnabled() != tc.updates {
			t.Fatalf("incorrect identity: %+v", tc)
		}
	}
}

func TestSelectedRejectsUnknown(t *testing.T) {
	before := variant
	t.Cleanup(func() { variant = before })
	variant = "unknown"
	if _, err := Selected(); err == nil {
		t.Fatal("unknown build identity accepted")
	}
}
