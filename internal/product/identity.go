// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

// Package product defines the build identity independently of application features.
package product

import "fmt"

var variant = "autobrr"

// Identity is an immutable product configuration.
type Identity struct {
	slug string
}

// Autobrr returns the legacy product identity.
func Autobrr() Identity { return Identity{slug: "autobrr"} }

// UFCManager returns the UFC Manager product identity.
func UFCManager() Identity { return Identity{slug: "ufc-manager"} }

// Selected validates the identity selected by the build's linker flags.
func Selected() (Identity, error) {
	switch variant {
	case "autobrr":
		return Autobrr(), nil
	case "ufc-manager":
		return UFCManager(), nil
	default:
		return Identity{}, fmt.Errorf("unknown product identity %q", variant)
	}
}

// Slug returns the configuration and artifact namespace.
func (i Identity) Slug() string {
	if i.slug == "" {
		return "autobrr"
	}
	return i.slug
}

// Name returns the product's display name.
func (i Identity) Name() string {
	if i.IsUFC() {
		return "UFC Manager"
	}
	return "autobrr"
}

// IsUFC reports whether UFC functionality belongs to this build.
func (i Identity) IsUFC() bool { return i.slug == "ufc-manager" }

// Port returns the default HTTP port.
func (i Identity) Port() int {
	if i.IsUFC() {
		return 7475
	}
	return 7474
}

// UpdatesEnabled reports whether this product has a configured update channel.
func (i Identity) UpdatesEnabled() bool { return !i.IsUFC() }

// EnvPrefix returns the isolated configuration environment namespace.
func (i Identity) EnvPrefix() string {
	if i.IsUFC() {
		return "UFC_MANAGER__"
	}
	return "AUTOBRR__"
}

// RegistryKey returns the per-user desktop settings namespace.
func (i Identity) RegistryKey() string {
	return `Software\` + i.Slug() + `\` + i.Slug() + `-desktop`
}
