// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"math"
	"time"

	"github.com/autobrr/autobrr/internal/ufc/acquisition"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/pkg/errors"
)

// Acquisition reads one retained journal entry, including dismissed entries.
func (r *Repository) Acquisition(ctx context.Context, id string) (acquisition.Job, error) {
	j, err := decodeAcquisition(r.db.QueryRowContext(ctx, "SELECT "+acquisitionColumns+" FROM ufc_acquisitions WHERE id=$1", id))
	if errors.Is(err, sql.ErrNoRows) {
		return j, errors.Wrap(catalogue.ErrInvalidEdit, "download no longer exists")
	}
	return j, err
}

// BindAcquisition commits a verified binding and its audit record atomically.
func (r *Repository) BindAcquisition(ctx context.Context, j acquisition.Job, actor, previous string) error {
	if j.RemoteID == "" || len(j.RemoteID) > 256 || len(j.Storage) > 8192 || len(j.Message) > 512 || math.IsNaN(j.Percent) || math.IsInf(j.Percent, 0) || j.Percent < 0 || j.Percent > 100 {
		return errors.Wrap(catalogue.ErrInvalidEdit, "invalid verified download status")
	}
	return r.transaction(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		result, err := tx.ExecContext(ctx, "UPDATE ufc_acquisitions SET revision=revision+1,state=$1,remote_id=$2,percent=$3,message=$4,storage=$5,updated_at=$6 WHERE id=$7 AND revision=$8 AND remote_id=$9 AND (remote_id='' OR remote_id=$2) AND state NOT IN ('completed','failed')", j.State, j.RemoteID, j.Percent, j.Message, j.Storage, now, j.ID, j.Revision, previous)
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
		var count int
		if err := tx.QueryRowContext(ctx, "SELECT COUNT(*) FROM ufc_acquisitions WHERE endpoint_key=$1 AND remote_id=$2 AND id<>$3", j.Endpoint, j.RemoteID, j.ID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.Wrap(catalogue.ErrInvalidEdit, "SAB job is already bound to another download")
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO ufc_acquisition_recovery(id,acquisition_id,actor,previous_remote_id,remote_id,previous_revision,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)", rand.Text(), j.ID, actor, previous, j.RemoteID, j.Revision, now); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "UPDATE release_action_status SET status='PUSH_APPROVED' WHERE id=(SELECT action_status_id FROM ufc_acquisitions WHERE id=$1)", j.ID)
		return err
	})
}
