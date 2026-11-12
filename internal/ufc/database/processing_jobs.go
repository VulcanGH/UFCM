// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/library"
	"github.com/autobrr/autobrr/internal/ufc/processing"
)

// EnqueueProcessingJob persists an explicit copy request and its file journal.
func (r *Repository) EnqueueProcessingJob(ctx context.Context, job processing.Job) (processing.Job, error) {
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		var existing string
		err := tx.QueryRowContext(ctx, "SELECT id FROM ufc_processing_jobs WHERE acquisition_id=$1", job.AcquisitionID).Scan(&existing)
		if err == nil {
			job.ID = existing
			return nil
		}
		if err != sql.ErrNoRows {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO ufc_processing_jobs(id,acquisition_id,event_id,component,destination_root,state,message,created_at,updated_at,cleanup_requested) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$8,$9)`, job.ID, job.AcquisitionID, job.EventID, job.Component, job.DestinationRoot, job.State, job.Message, job.CreatedAt, job.CleanupRequested)
		if err != nil {
			return err
		}
		for _, file := range job.Files {
			if _, err = tx.ExecContext(ctx, `INSERT INTO ufc_processing_files(job_id,source_path,destination_path,stage_path,source_size,state,original_destination,component) VALUES($1,$2,$3,$4,$5,'queued',$3,$6)`, job.ID, file.Source, file.Destination, file.Stage, file.Size, file.Component); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return processing.Job{}, err
	}
	return r.ProcessingJob(ctx, job.ID)
}

// ProcessingJob reads a job with its persisted file evidence.
func (r *Repository) ProcessingJob(ctx context.Context, id string) (processing.Job, error) {
	var job processing.Job
	err := r.db.QueryRowContext(ctx, `SELECT id,acquisition_id,event_id,component,destination_root,state,resolution,message,created_at,updated_at,cleanup_requested,cleanup_state,cleanup_message FROM ufc_processing_jobs WHERE id=$1`, id).Scan(&job.ID, &job.AcquisitionID, &job.EventID, &job.Component, &job.DestinationRoot, &job.State, &job.Resolution, &job.Message, &job.CreatedAt, &job.UpdatedAt, &job.CleanupRequested, &job.CleanupState, &job.CleanupMessage)
	if err != nil {
		return job, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT source_path,component,destination_path,stage_path,original_destination,source_size,copied_bytes,state,source_sha256,destination_sha256,message FROM ufc_processing_files WHERE job_id=$1 ORDER BY source_path`, id)
	if err != nil {
		return job, err
	}
	defer rows.Close()
	job.Files = []processing.JobFile{}
	for rows.Next() {
		var file processing.JobFile
		if err := rows.Scan(&file.Source, &file.Component, &file.Destination, &file.Stage, &file.OriginalDestination, &file.Size, &file.CopiedBytes, &file.State, &file.SourceSHA256, &file.DestinationSHA256, &file.Message); err != nil {
			return job, err
		}
		job.Files = append(job.Files, file)
	}
	return job, rows.Err()
}

// ProcessingJobs lists recent copy jobs without exposing unrelated downloads.
func (r *Repository) ProcessingJobs(ctx context.Context) ([]processing.Job, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM ufc_processing_jobs ORDER BY created_at DESC LIMIT 50`)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	jobs := make([]processing.Job, 0, len(ids))
	for _, id := range ids {
		job, err := r.ProcessingJob(ctx, id)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// ClaimProcessingCleanup marks one verified job before any source removal.
func (r *Repository) ClaimProcessingCleanup(ctx context.Context) (processing.Job, bool, error) {
	var id string
	err := r.transaction(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT id FROM ufc_processing_jobs WHERE state='completed' AND cleanup_requested=TRUE AND cleanup_state='pending' ORDER BY updated_at LIMIT 1`).Scan(&id); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE ufc_processing_jobs SET cleanup_state='claimed',updated_at=$1 WHERE id=$2 AND state='completed' AND cleanup_state='pending' AND NOT EXISTS (SELECT 1 FROM ufc_processing_files WHERE job_id=$2 AND state<>'verified')`, time.Now().UTC().Format(time.RFC3339Nano), id)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return sql.ErrNoRows
		}
		return nil
	})
	if err == sql.ErrNoRows {
		return processing.Job{}, false, nil
	}
	if err != nil {
		return processing.Job{}, false, err
	}
	job, err := r.ProcessingJob(ctx, id)
	return job, err == nil, err
}

// FinishProcessingCleanup records the outcome without automatically retrying uncertainty.
func (r *Repository) FinishProcessingCleanup(ctx context.Context, id, state, message string) error {
	if state != "done" && state != "needs_review" {
		return sql.ErrNoRows
	}
	return r.transaction(ctx,func(tx *sql.Tx)error{
	result, err := tx.ExecContext(ctx, `UPDATE ufc_processing_jobs SET cleanup_state=$1,cleanup_message=$2,updated_at=$3 WHERE id=$4 AND cleanup_state='claimed'`, state, message, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	if state=="needs_review"{return queueProcessingNotice(ctx,tx,id,"UFC_ATTENTION","Source cleanup needs review; Library copies are retained")}
	return nil
	})
}

// InterruptProcessingCleanup leaves interrupted removals for manual inspection.
func (r *Repository) InterruptProcessingCleanup(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ufc_processing_jobs SET cleanup_state='needs_review',cleanup_message='Cleanup was interrupted; inspect completed download before any further action',updated_at=$1 WHERE cleanup_state='claimed'`, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

// RetryProcessingCleanup requeues a reviewed cleanup without repeating the copy.
func (r *Repository) RetryProcessingCleanup(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE ufc_processing_jobs SET cleanup_state='pending',cleanup_message='',updated_at=$1 WHERE id=$2 AND state='completed' AND cleanup_requested=TRUE AND cleanup_state='needs_review' AND NOT EXISTS (SELECT 1 FROM ufc_processing_files WHERE job_id=$2 AND state<>'verified')`, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

// RecoverableProcessingJobs lists unfinished published evidence for restart reconciliation.
func (r *Repository) RecoverableProcessingJobs(ctx context.Context) ([]processing.Job, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT DISTINCT j.id FROM ufc_processing_jobs j JOIN ufc_processing_files f ON f.job_id=j.id WHERE j.state='needs_review' AND j.resolution='' AND f.state IN ('staging','published') ORDER BY j.id LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	jobs := make([]processing.Job, 0, len(ids))
	for _, id := range ids {
		job, err := r.ProcessingJob(ctx, id)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// InterruptProcessingJobs leaves uncertain work for review after a restart.
func (r *Repository) InterruptProcessingJobs(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ufc_processing_jobs SET state='needs_review',message='Processing interrupted; inspect staged and published files before retrying',updated_at=$1 WHERE state='running'`, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

// ClaimProcessingJob atomically takes one queued job for a single bounded worker.
func (r *Repository) ClaimProcessingJob(ctx context.Context) (processing.Job, bool, error) {
	var id string
	err := r.db.QueryRowContext(ctx, `SELECT id FROM ufc_processing_jobs WHERE state='queued' ORDER BY created_at LIMIT 1`).Scan(&id)
	if err == sql.ErrNoRows {
		return processing.Job{}, false, nil
	}
	if err != nil {
		return processing.Job{}, false, err
	}
	result, err := r.db.ExecContext(ctx, `UPDATE ufc_processing_jobs SET state='running',updated_at=$1 WHERE id=$2 AND state='queued'`, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return processing.Job{}, false, err
	}
	n, err := result.RowsAffected()
	if err != nil || n == 0 {
		return processing.Job{}, false, err
	}
	job, err := r.ProcessingJob(ctx, id)
	return job, err == nil, err
}

// SetProcessingFile records a durable checkpoint before or after a file operation.
func (r *Repository) SetProcessingFile(ctx context.Context, id, source, state, sourceHash, destinationHash string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE ufc_processing_files SET state=$1,source_sha256=$2,destination_sha256=$3 WHERE job_id=$4 AND source_path=$5`, state, sourceHash, destinationHash, id, source)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

// SetProcessingProgress stores bounded copy progress for the Downloads view.
func (r *Repository) SetProcessingProgress(ctx context.Context, id, source string, copied int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ufc_processing_files SET copied_bytes=$1 WHERE job_id=$2 AND source_path=$3 AND state='staging'`, copied, id, source)
	return err
}

// RetryProcessingJob requeues only an unresolved job.
func (r *Repository) RetryProcessingJob(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE ufc_processing_jobs SET state='queued',message='Retry queued after explicit review',updated_at=$1 WHERE id=$2 AND state='needs_review' AND resolution=''`, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

// ResolveProcessingJob records an explicit skip or cancellation without media changes.
func (r *Repository) ResolveProcessingJob(ctx context.Context, id, resolution, message string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE ufc_processing_jobs SET state='needs_review',resolution=$1,message=$2,updated_at=$3 WHERE id=$4 AND state IN ('queued','needs_review') AND resolution=''`, resolution, message, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return sql.ErrNoRows
	}
	return nil
}

// CancelRunningProcessingJob records a user cancellation after the worker stops.
func (r *Repository) CancelRunningProcessingJob(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE ufc_processing_jobs SET state='needs_review',message='Cancelled; inspect staged or published files before retrying',updated_at=$1 WHERE id=$2 AND state='running'`, time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}

// SetProcessingDestination records an authorised keep-both path before retrying.
func (r *Repository) SetProcessingDestination(ctx context.Context, id, destination string) error {
	return r.transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE ufc_processing_jobs SET state='queued',message='Keep-both copy queued',updated_at=$1 WHERE id=$2 AND state='needs_review' AND resolution=''`, time.Now().UTC().Format(time.RFC3339Nano), id)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil || n != 1 {
			return sql.ErrNoRows
		}
		_, err = tx.ExecContext(ctx, `UPDATE ufc_processing_files SET original_destination=destination_path WHERE job_id=$1 AND original_destination=''`, id)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE ufc_processing_files SET destination_path=$1 WHERE job_id=$2`, destination, id)
		return err
	})
}

// ReviewProcessingJob records a failure without removing staged or published media.
func (r *Repository) ReviewProcessingJob(ctx context.Context, id, message string) error {
	return r.transaction(ctx,func(tx *sql.Tx)error{
		result,err:=tx.ExecContext(ctx, `UPDATE ufc_processing_jobs SET state='needs_review',message=$1,updated_at=$2 WHERE id=$3 AND state='running'`, message, time.Now().UTC().Format(time.RFC3339Nano), id)
		if err!=nil{return err};n,err:=result.RowsAffected();if err!=nil{return err};if n==0{return nil}
		return queueProcessingNotice(ctx,tx,id,"UFC_ATTENTION","Library copy needs review; see UFC Downloads")
	})
}

// CompleteProcessingJob records verified media in the Library and completes its journal atomically.
func (r *Repository) CompleteProcessingJob(ctx context.Context, job processing.Job, size int64, modified, hash string) error {
	return r.transaction(ctx, func(tx *sql.Tx) error {
		var rootID string
		if err := tx.QueryRowContext(ctx, `SELECT r.id FROM ufc_library_roots r JOIN ufc_library_settings s ON s.id=1 WHERE r.path=$1 AND r.enabled=TRUE AND r.state='available' AND s.running=FALSE`, job.DestinationRoot).Scan(&rootID); err != nil {
			return err
		}
		file := job.Files[0]
		relative := filepath.ToSlash(filepath.Clean(filepath.FromSlash(file.Destination)))
		keyPath := filepath.Clean(filepath.FromSlash(relative))
		if runtime.GOOS == "windows" {
			keyPath = strings.ToLower(keyPath)
		}
		fileID := library.Key(rootID + "\x00" + keyPath)
		now := time.Now().UTC().Format(time.RFC3339Nano)
		component := file.Component
		if component == "" {
			component = job.Component
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO ufc_library_files(id,root_id,relative_path,size,modified,event_id,component,resolution,state,reason,manual,present,group_key,last_seen,detected_event,detected_component,detected_resolution,detected_state,detected_reason,actor) VALUES($1,$2,$3,$4,$5,$6,$7,'unknown','confirmed','Verified native copy',TRUE,TRUE,$8,$9,$6,$7,'unknown','confirmed','Verified native copy','processing') ON CONFLICT(id) DO NOTHING`, fileID, rootID, relative, size, modified, job.EventID, component, "event:"+job.EventID, now)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			var existingRoot, existingPath, existingModified, existingEvent, existingComponent, existingState string
			var existingSize int64
			var present bool
			if err := tx.QueryRowContext(ctx, `SELECT root_id,relative_path,size,modified,event_id,component,state,present FROM ufc_library_files WHERE id=$1`, fileID).Scan(&existingRoot, &existingPath, &existingSize, &existingModified, &existingEvent, &existingComponent, &existingState, &present); err != nil {
				return err
			}
			if existingRoot != rootID || existingPath != relative || existingSize != size || existingModified != modified || existingEvent != job.EventID || existingComponent != component || existingState != "confirmed" || !present {
				return sql.ErrNoRows
			}
		}
		result, err = tx.ExecContext(ctx, `UPDATE ufc_processing_files SET state='verified',source_sha256=$1,destination_sha256=$1 WHERE job_id=$2 AND source_path=$3 AND state='published'`, hash, job.ID, file.Source)
		if err != nil {
			return err
		}
		n, err = result.RowsAffected()
		if err != nil || n != 1 {
			return sql.ErrNoRows
		}
		var remaining int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM ufc_processing_files WHERE job_id=$1 AND state<>'verified'`, job.ID).Scan(&remaining); err != nil {
			return err
		}
		if remaining > 0 {
			_, err = tx.ExecContext(ctx, `UPDATE ufc_processing_jobs SET updated_at=$1 WHERE id=$2 AND state IN ('running','needs_review') AND resolution=''`, now, job.ID)
			return err
		}
		result, err = tx.ExecContext(ctx, `UPDATE ufc_processing_jobs SET state='completed',message='Verified copy recorded in Library',cleanup_state=CASE WHEN cleanup_requested THEN 'pending' ELSE 'disabled' END,updated_at=$1 WHERE id=$2 AND state IN ('running','needs_review') AND resolution=''`, now, job.ID)
		if err != nil {
			return err
		}
		n, err = result.RowsAffected()
		if err != nil || n != 1 {
			return sql.ErrNoRows
		}
		if err:=archiveCompletedEvent(ctx,tx,job.EventID);err!=nil{return err}
		return queueProcessingNotice(ctx,tx,job.ID,"UFC_COPY_COMPLETED","Verified copy recorded in Library")
	})
}
