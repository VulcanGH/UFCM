// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later
//go:build !windows

package updater

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Lock protects a profile on supported non-Windows application hosts.
func Lock(root string) (func(), error) { return lockFile(filepath.Join(root, "install.lock")) }

// RuntimeLock keeps profile exclusion independent of launcher lifetime.
func RuntimeLock(root string) (func(), error) { return lockFile(filepath.Join(root, "runtime.lock")) }
func legacyProfileIdle(string) error          { return nil }
func lockFile(name string) (func(), error) {
	f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return func() { _ = f.Close() }, nil
}

// PrivateDirectory limits runtime and backup access to the owning user.
func PrivateDirectory(root string) error { return os.Chmod(root, 0700) }

func reparse(os.FileInfo) bool          { return false }
func replaceFile(from, to string) error { return os.Rename(from, to) }
