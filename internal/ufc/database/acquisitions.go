// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/acquisition"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/pkg/errors"
)

const acquisitionColumns = "id,revision,event_id,component,title,source,client_id,client_name,category,state,remote_id,percent,message,release_id,release_key,endpoint_key,storage,actor,created_at,updated_at,decision_json"

func decodeAcquisition(row scanner) (acquisition.Job, error) {
	var j acquisition.Job
	var decision string
	err := row.Scan(&j.ID, &j.Revision, &j.EventID, &j.Component, &j.Title, &j.Source, &j.ClientID, &j.ClientName, &j.Category, &j.State, &j.RemoteID, &j.Percent, &j.Message, &j.ReleaseID, &j.Key, &j.Endpoint, &j.Storage, &j.Actor, &j.CreatedAt, &j.UpdatedAt, &decision)
	if err == nil {
		err = json.Unmarshal([]byte(decision), &j.Decision)
	}
	return j, err
}

// DismissAcquisition removes a card without deleting its journal or remote job.
func (r *Repository) DismissAcquisition(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "UPDATE ufc_acquisitions SET dismissed=TRUE,revision=revision+1 WHERE id=$1", id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.Wrap(catalogue.ErrInvalidEdit, "download no longer exists")
	}
	return nil
}

// ReserveAcquisition atomically reserves intent and projects safe release/push history.
func (r *Repository) ReserveAcquisition(ctx context.Context, j acquisition.Job) (acquisition.Job, bool, error) {
	created := false
	decision, err := json.Marshal(j.Decision)
	if err != nil {
		return j, false, err
	}
	if j.Size > math.MaxInt64 {
		return j, false, errors.Wrap(catalogue.ErrInvalidEdit, "release size exceeds supported range")
	}
	err = r.transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `INSERT INTO ufc_acquisitions(id,event_id,component,title,source,client_id,client_name,category,state,release_id,action_status_id,release_key,endpoint_key,actor,created_at,updated_at,decision_json) VALUES($1,$2,$3,$4,$5,$6,$7,$8,'submitting',0,0,$9,$10,$11,$12,$12,$13) ON CONFLICT DO NOTHING`, j.ID, j.EventID, j.Component, j.Title, j.Source, j.ClientID, j.ClientName, j.Category, j.Key, j.Endpoint, j.Actor, j.CreatedAt, string(decision))
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			j, err = decodeAcquisition(tx.QueryRowContext(ctx, "SELECT "+acquisitionColumns+" FROM ufc_acquisitions WHERE id=$4 OR (release_key=$1 AND state<>'failed') OR (event_id=$2 AND component=$3 AND state<>'failed') ORDER BY created_at LIMIT 1", j.Key, j.EventID, j.Component, j.ID))
			return err
		}
		lock := ""
		if r.driver == "postgres" {
			lock = " FOR SHARE"
		}
		var eventRevision, settingsRevision int64
		if err := tx.QueryRowContext(ctx, "SELECT revision FROM ufc_events WHERE id=$1"+lock, j.EventID).Scan(&eventRevision); err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx, "SELECT revision FROM ufc_catalogue_settings WHERE id=1"+lock).Scan(&settingsRevision); err != nil {
			return err
		}
		if eventRevision != j.EventRevision || settingsRevision != j.SettingsRevision {
			return catalogue.ErrRevisionConflict
		}
		if j.Decision != nil && j.Decision.Automated {
			if err := guardAutomaticAcquisition(ctx, tx, r.driver, j); err != nil {
				return err
			}
		}
		if err := guardLibraryCoverage(ctx, tx, r.driver, j.EventID, j.Component); err != nil {
			if j.Decision != nil && j.Decision.Automated {
				return acquisition.ErrAutomaticDeferred
			}
			return err
		}
		workflow := "UFC manual"
		if j.Decision != nil && j.Decision.Automated {
			workflow = "UFC automatic"
		}
		indexer := j.Indexer
		if indexer == "" {
			indexer = j.Source
		}
		err = tx.QueryRowContext(ctx, `INSERT INTO "release"(filter_status,indexer,filter,protocol,implementation,timestamp,torrent_name,normalized_hash,size,title,sub_title,category,season,episode,year,month,day,resolution,source,codec,container,hdr,audio,audio_channels,release_group,region,language,edition,cut,hybrid,proper,repack,website,media_processing,type,origin,info_url,download_url,group_id,torrent_id,pre_time,uploader) VALUES('FILTER_APPROVED',$1,$9,'usenet','NEWZNAB',$2,$3,$4,$5,$3,'','',0,0,0,0,0,$6,$7,$8,'','','','','','','','','',FALSE,FALSE,FALSE,'','','',$9,'','','','','','') RETURNING id`, indexer, j.CreatedAt, j.Title, j.Key, j.Size, j.Resolution, j.MediaSource, j.Codec, workflow).Scan(&j.ReleaseID)
		if err != nil {
			return err
		}
		var actionID int64
		err = tx.QueryRowContext(ctx, `INSERT INTO release_action_status(status,action,type,client,filter,release_id,timestamp) VALUES('PENDING',$5,'SABNZBD',$1,$4,$2,$3) RETURNING id`, j.ClientName, j.ReleaseID, j.CreatedAt, workflow, workflow+" download").Scan(&actionID)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE ufc_acquisitions SET release_id=$1,action_status_id=$2 WHERE id=$3", j.ReleaseID, actionID, j.ID)
		j.Revision = 1
		j.State = "submitting"
		j.UpdatedAt = j.CreatedAt
		created = true
		return err
	})
	return j, created, errors.Wrap(err, "reserve manual download")
}

// SaveAcquisition updates tracked state and push history with optimistic concurrency.
func (r *Repository) SaveAcquisition(ctx context.Context, j acquisition.Job) error {
	if math.IsNaN(j.Percent) || math.IsInf(j.Percent, 0) {
		j.Percent = 0
	}
	j.Percent = max(0, min(100, j.Percent))
	if len(j.Message) > 512 || len(j.RemoteID) > 256 || len(j.Storage) > 8192 {
		return errors.New("invalid download status")
	}
	return r.transaction(ctx, func(tx *sql.Tx) error {
		pending := false
		if j.State == "completed" && j.RemoteID != "" && j.Storage != "" {
			var raw string
			if err := tx.QueryRowContext(ctx, "SELECT settings_json FROM ufc_processing_settings WHERE id=1").Scan(&raw); err != nil {
				return err
			}
			var settings struct {
				Auto     bool   `json:"auto_process_completed"`
				ClientID int32  `json:"sab_client_id"`
				Category string `json:"sab_category"`
			}
			if err := json.Unmarshal([]byte(raw), &settings); err != nil {
				return err
			}
			pending = settings.Auto && settings.ClientID == j.ClientID && settings.Category == j.Category
		}
		result, err := tx.ExecContext(ctx, "UPDATE ufc_acquisitions SET revision=revision+1,state=$1,remote_id=$2,percent=$3,message=$4,storage=$5,updated_at=$6,processing_auto_pending=$9 WHERE id=$7 AND revision=$8 AND state NOT IN ('completed','failed')", j.State, j.RemoteID, j.Percent, j.Message, j.Storage, time.Now().UTC().Format(time.RFC3339Nano), j.ID, j.Revision, pending)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return catalogue.ErrRevisionConflict
		}
		status := "PENDING"
		if j.RemoteID != "" {
			status = "PUSH_APPROVED"
		} else if j.State == "failed" {
			status = "PUSH_ERROR"
		}
		_, err = tx.ExecContext(ctx, "UPDATE release_action_status SET status=$1 WHERE id=(SELECT action_status_id FROM ufc_acquisitions WHERE id=$2)", status, j.ID)
		if err != nil {
			return err
		}
		if j.State == "completed" {
			return queueUFCNotice(ctx, tx, "download-completed:"+j.ID, "UFC_DOWNLOAD_COMPLETED", j.Title, "SAB download completed")
		}
		if j.State == "failed" || j.State == "needs_review" || j.State == "missing" {
			return queueUFCNotice(ctx, tx, "download-attention:"+j.ID+":"+j.State, "UFC_ATTENTION", j.Title, "Download needs attention; see UFC Downloads")
		}
		return nil
	})
}

// PendingProcessingHandoffs returns completed, owned downloads awaiting a copy job.
func (r *Repository) PendingProcessingHandoffs(ctx context.Context) ([]acquisition.Job, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT "+acquisitionColumns+" FROM ufc_acquisitions WHERE processing_auto_pending=TRUE AND state='completed' ORDER BY updated_at,id LIMIT 50")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []acquisition.Job{}
	for rows.Next() {
		job, err := decodeAcquisition(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// FinishProcessingHandoff clears a pending flag only after durable job creation.
func (r *Repository) FinishProcessingHandoff(ctx context.Context, id string) (bool, error) {
	result, err := r.db.ExecContext(ctx, "UPDATE ufc_acquisitions SET processing_auto_pending=FALSE WHERE id=$1 AND processing_auto_pending=TRUE AND state='completed' AND EXISTS (SELECT 1 FROM ufc_processing_jobs WHERE acquisition_id=$1)", id)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n == 1, err
}

// Acquisitions reads recent history or a fair bounded batch of active records.
func (r *Repository) Acquisitions(ctx context.Context, active bool) ([]acquisition.Job, error) {
	query := "SELECT " + acquisitionColumns + " FROM ufc_acquisitions WHERE dismissed=FALSE ORDER BY created_at DESC LIMIT 100"
	if active {
		query = "SELECT " + acquisitionColumns + " FROM ufc_acquisitions WHERE state NOT IN ('completed','failed') ORDER BY updated_at,id LIMIT 50"
	}
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []acquisition.Job{}
	for rows.Next() {
		j, err := decodeAcquisition(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}
