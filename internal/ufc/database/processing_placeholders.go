// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package database

import (
	"context"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/processing"
)

const placeholderTimeFormat = "2006-01-02T15:04:05.000000000Z"

func (r *Repository) PendingPlaceholders(ctx context.Context, now time.Time) ([]processing.PlaceholderTask, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT j.id,f.source_path,j.destination_root,f.destination_path,
		COALESCE(NULLIF(f.component,''),j.component),f.source_size
		FROM ufc_processing_files f JOIN ufc_processing_jobs j ON j.id=f.job_id
		JOIN ufc_library_roots r ON r.path=j.destination_root AND r.enabled=TRUE AND r.state='available'
		WHERE f.placeholder_due<>'' AND f.placeholder_due<=$1
		ORDER BY f.placeholder_due,j.id,f.source_path LIMIT 32`, now.UTC().Format(placeholderTimeFormat))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tasks []processing.PlaceholderTask
	for rows.Next() {
		var task processing.PlaceholderTask
		if err := rows.Scan(&task.JobID, &task.Source, &task.Root, &task.Destination, &task.Component, &task.Size); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (r *Repository) FinishPlaceholder(ctx context.Context, task processing.PlaceholderTask, retry time.Time) error {
	due := ""
	if !retry.IsZero() {
		due = retry.UTC().Format(placeholderTimeFormat)
	}
	_, err := r.db.ExecContext(ctx, `UPDATE ufc_processing_files SET placeholder_due=$1 WHERE job_id=$2 AND source_path=$3`, due, task.JobID, task.Source)
	return err
}
