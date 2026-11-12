// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package cli

import (
	"context"

	"github.com/autobrr/autobrr/pkg/dbsnapshot"

	"github.com/spf13/cobra"
)

// CommandDbBackup snapshots an existing SQLite database without starting or migrating the app.
func CommandDbBackup() *cobra.Command {
	var source, destination string
	cmd := &cobra.Command{Use: "backup", Short: "Create a verified SQLite snapshot without replacing existing files", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error { return backupSQLite(cmd.Context(), source, destination) }}
	cmd.Flags().StringVar(&source, "db-path", "", "existing SQLite database file")
	cmd.Flags().StringVar(&destination, "output", "", "new backup file (must not already exist)")
	_ = cmd.MarkFlagRequired("db-path")
	_ = cmd.MarkFlagRequired("output")
	return cmd
}

func backupSQLite(ctx context.Context, source, destination string) error {
	return dbsnapshot.SQLite(ctx, source, destination)
}
