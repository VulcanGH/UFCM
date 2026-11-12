// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package updater

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// ValidateDatabaseStorage reads only root metadata before the launcher creates runtime files.
func ValidateDatabaseStorage(ctx context.Context, driver, dsn, root string) error {
	if err := SafeDirectory(root); err != nil {
		return err
	}
	sqlDriver := driver
	if driver == "sqlite" {
		info, err := os.Lstat(dsn)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil || !info.Mode().IsRegular() || reparse(info) {
			return fmt.Errorf("database path cannot be verified for launcher storage")
		}
		if err := Disjoint(root, filepath.Dir(dsn)); err != nil {
			return err
		}
		dsn = "file:" + url.PathEscape(filepath.ToSlash(dsn)) + "?mode=ro"
	} else if driver != "postgres" {
		return fmt.Errorf("unsupported database")
	}
	db, err := sql.Open(sqlDriver, dsn)
	if err != nil {
		return fmt.Errorf("cannot validate launcher storage")
	}
	defer db.Close()
	countQuery := `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=$1`
	if driver == "postgres" {
		countQuery = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=$1`
	}
	var count int
	if err = db.QueryRowContext(ctx, countQuery, "ufc_library_roots").Scan(&count); err != nil {
		return fmt.Errorf("cannot read library root metadata")
	}
	if count > 0 {
		rows, err := db.QueryContext(ctx, "SELECT path FROM ufc_library_roots")
		if err != nil {
			return err
		}
		for rows.Next() {
			var media string
			if err = rows.Scan(&media); err != nil {
				rows.Close()
				return err
			}
			if err = Disjoint(root, media); err != nil {
				rows.Close()
				return err
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	if err = db.QueryRowContext(ctx, countQuery, "ufc_processing_settings").Scan(&count); err != nil {
		return fmt.Errorf("cannot read processing root metadata")
	}
	if count > 0 {
		var raw string
		if err = db.QueryRowContext(ctx, "SELECT settings_json FROM ufc_processing_settings WHERE id=1").Scan(&raw); err != nil {
			return err
		}
		var settings struct {
			Sources     []string `json:"source_roots"`
			Destination string   `json:"destination_root"`
		}
		if err = json.Unmarshal([]byte(raw), &settings); err != nil {
			return err
		}
		for _, media := range append(settings.Sources, settings.Destination) {
			if media != "" {
				if err = Disjoint(root, media); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
