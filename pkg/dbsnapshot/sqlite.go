// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

// Package dbsnapshot creates verified SQLite recovery copies without migrating the source.
package dbsnapshot

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"

	"github.com/autobrr/autobrr/pkg/errors"

	_ "modernc.org/sqlite"
)

// SQLite writes a consistent snapshot to a new file and verifies it before returning.
func SQLite(ctx context.Context, source, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	source, err := filepath.Abs(source)
	if err != nil {
		return err
	}
	destination, err = filepath.Abs(destination)
	if err != nil {
		return err
	}
	info, err := os.Stat(source)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("backup source must be an existing SQLite file")
	}
	db, err := sql.Open("sqlite", "file:"+url.PathEscape(filepath.ToSlash(source))+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return errors.Wrap(err, "could not open backup source read-only")
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.Wrap(err, "backup destination must be a new file")
	}
	if err := output.Close(); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", destination); err != nil {
		return errors.Wrap(err, "snapshot failed; incomplete output retained for inspection")
	}
	copyDB, err := sql.Open("sqlite", "file:"+url.PathEscape(filepath.ToSlash(destination))+"?mode=ro")
	if err != nil {
		return err
	}
	defer copyDB.Close()
	var check string
	if err := copyDB.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&check); err != nil {
		return err
	}
	if check != "ok" {
		return errors.New("backup verification failed")
	}
	return nil
}
