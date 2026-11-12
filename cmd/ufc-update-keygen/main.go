// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/autobrr/autobrr/internal/ufc/updater"
)

func main() {
	root, err := os.UserConfigDir()
	if err == nil {
		err = generate(filepath.Join(root, "UFC Manager Signing"))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(directory string) error {
	if err := updater.SafeDirectory(filepath.Dir(directory)); err != nil {
		return err
	}
	if err := os.Mkdir(directory, 0700); err != nil {
		return fmt.Errorf("signing directory must be new; existing keys will not be replaced: %w", err)
	}
	if err := updater.PrivateDirectory(directory); err != nil {
		return fmt.Errorf("could not protect signing directory: %w", err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	for _, item := range []struct {
		name string
		key  []byte
	}{{"private-key.txt", private}, {"public-key.txt", public}} {
		file, err := os.OpenFile(filepath.Join(directory, item.name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, writeErr := file.WriteString(base64.StdEncoding.EncodeToString(item.key))
		syncErr := file.Sync()
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		if syncErr != nil {
			return syncErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	fmt.Println("Signing keys saved in:", directory)
	fmt.Println("Keep private-key.txt secret. Neither key was printed.")
	return nil
}
