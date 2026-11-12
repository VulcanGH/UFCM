// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

func removeOwnedFlat(root, dir string, allowed map[string]bool) error {
	base, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	target, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if filepath.Dir(target) != base {
		return fmt.Errorf("cleanup target is outside the owned runtime store")
	}
	if err = SafeDirectory(target); err != nil {
		return err
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		info, err := os.Lstat(filepath.Join(target, entry.Name()))
		if err != nil || !allowed[entry.Name()] || !info.Mode().IsRegular() || reparse(info) {
			return fmt.Errorf("uncertain runtime directory retained")
		}
	}
	for _, entry := range entries {
		if err = os.Remove(filepath.Join(target, entry.Name())); err != nil {
			return err
		}
	}
	return os.Remove(target)
}

// Prune removes only verified unreferenced runtime files while the installation is exclusively owned.
func (l *Launcher) Prune(state Bootstrap) error {
	root := filepath.Join(l.Root, "versions")
	if err := SafeDirectory(root); err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	keep := map[string]bool{state.Active: true, state.Previous: true, state.Candidate: true, state.FailedID: true}
	var request Request
	if err := ReadJSON(filepath.Join(l.Root, "request.json"), &request); err == nil {
		keep[ID(request.Envelope)] = true
	} else if !os.IsNotExist(err) {
		return nil
	}
	for _, entry := range entries {
		if keep[entry.Name()] || !digest(entry.Name()) {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		var envelope Envelope
		if err = ReadJSON(filepath.Join(dir, "release.json"), &envelope); err != nil || ID(envelope) != entry.Name() {
			continue
		}
		manifest, verifyErr := l.Trust.Verify(envelope, time.Now(), 0, false)
		if verifyErr != nil {
			continue
		}
		if err = NewClient(l.Trust).VerifyRuntime(dir, envelope, 0); err != nil {
			continue
		}
		allowed := map[string]bool{"release.json": true}
		for _, file := range manifest.Files {
			allowed[file.Name] = true
		}
		if err = removeOwnedFlat(root, dir, allowed); err != nil {
			return err
		}
	}
	return nil
}
