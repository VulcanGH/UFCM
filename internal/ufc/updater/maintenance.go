// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package updater

import (
	"fmt"
	"os"
	"path/filepath"
)

const maintenanceFile = "installer-maintenance.json"

type installerLease struct {
	ID      string `json:"id"`
	Profile string `json:"profile"`
}

// CheckMaintenance refuses ambiguous or unfinished installer transactions.
func CheckMaintenance(root string) error {
	_, err := os.Lstat(filepath.Join(root, maintenanceFile))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("Windows Installer maintenance is pending; complete or repair that installation first")
}

// BeginMaintenance atomically excludes launcher and direct profile starts during MSI work.
func BeginMaintenance(root, profile, id string) error {
	if id == "" || len(id) > 64 {
		return fmt.Errorf("invalid installer lease")
	}
	if err := SafeDirectory(root); err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	if err := PrivateDirectory(root); err != nil {
		return err
	}
	release, err := Lock(root)
	if err != nil {
		return fmt.Errorf("quit UFC Manager before modifying its installation")
	}
	defer release()
	var state Bootstrap
	if err := ReadJSON(filepath.Join(root, "bootstrap-state.json"), &state); err == nil && state.Config != "" {
		profile = state.Config
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := legacyProfileIdle(profile); err != nil {
		return err
	}
	if err := SafeDirectory(profile); err != nil {
		return err
	}
	if err := os.MkdirAll(profile, 0700); err != nil {
		return err
	}
	profileRelease, err := Lock(profile)
	if err != nil {
		return fmt.Errorf("quit UFC Manager before modifying its installation")
	}
	defer profileRelease()
	runtimeRelease, err := RuntimeLock(profile)
	if err != nil {
		return fmt.Errorf("quit UFC Manager before modifying its installation")
	}
	defer runtimeRelease()
	lease := installerLease{ID: id, Profile: profile}
	for _, dir := range []string{root, profile} {
		var previous installerLease
		if err := ReadJSON(filepath.Join(dir, maintenanceFile), &previous); err == nil && previous.ID != id {
			return fmt.Errorf("complete or repair the previous installer transaction first")
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err = WriteJSON(filepath.Join(root, maintenanceFile), lease); err != nil {
		return err
	}
	return WriteJSON(filepath.Join(profile, maintenanceFile), lease)
}

// EndMaintenance clears only a matching completed installer lease, retaining application data.
func EndMaintenance(root, id string) error {
	if err := SafeDirectory(root); err != nil {
		return err
	}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	}
	release, err := Lock(root)
	if err != nil {
		return err
	}
	defer release()
	var lease installerLease
	if err = ReadJSON(filepath.Join(root, maintenanceFile), &lease); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if lease.ID != id {
		return fmt.Errorf("installer lease does not match")
	}
	if err = SafeDirectory(lease.Profile); err != nil {
		return err
	}
	profileRelease, err := Lock(lease.Profile)
	if err != nil {
		return err
	}
	defer profileRelease()
	var profileLease installerLease
	if err = ReadJSON(filepath.Join(lease.Profile, maintenanceFile), &profileLease); err == nil {
		if profileLease.ID != id {
			return fmt.Errorf("profile lease does not match")
		}
		if err = os.Remove(filepath.Join(lease.Profile, maintenanceFile)); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Remove(filepath.Join(root, maintenanceFile))
}
