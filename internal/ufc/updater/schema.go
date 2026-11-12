// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// SourceSchemaDigest hashes migration names and contents in deterministic order during release builds.
func SourceSchemaDigest(root string) (string, error) {
	var paths []string
	for _, area := range []string{"internal/database/migrations", "internal/ufc/database/migrations"} {
		for _, engine := range []string{"sqlite", "postgres"} {
			entries, err := filepath.Glob(filepath.Join(root, area, engine, "*.sql"))
			if err != nil {
				return "", err
			}
			if len(entries) == 0 {
				return "", fmt.Errorf("migration sources missing")
			}
			paths = append(paths, entries...)
		}
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, path := range paths {
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return "", err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		h.Write([]byte(filepath.ToSlash(relative)))
		h.Write([]byte{0})
		h.Write(data)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
