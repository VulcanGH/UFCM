// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// Lock holds an installation-wide file handle across Windows sessions and child lifetimes.
func Lock(root string) (func(), error) {
	return lockFile(root + `\install.lock`)
}

func legacyProfileIdle(profile string) error {
	absolute, err := filepath.Abs(profile)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(absolute))))
	name, err := windows.UTF16PtrFromString(`Local\autobrr-` + hex.EncodeToString(sum[:16]))
	if err != nil {
		return err
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if handle != 0 {
		_ = windows.CloseHandle(handle)
	}
	if err != nil {
		return fmt.Errorf("quit UFC Manager before modifying its installation")
	}
	return nil
}

// RuntimeLock prevents another process opening this profile if its launcher dies.
func RuntimeLock(root string) (func(), error) { return lockFile(root + `\runtime.lock`) }

func lockFile(name string) (func(), error) {
	path, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(path, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("installation is already running or unavailable: %w", err)
	}
	return func() { _ = windows.CloseHandle(h) }, nil
}

// PrivateDirectory limits inherited runtime and backup access to this user and SYSTEM.
func PrivateDirectory(root string) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(root, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

func reparse(info os.FileInfo) bool {
	data, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && data.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0
}
func replaceFile(from, to string) error {
	a, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	b, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(a, b, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
