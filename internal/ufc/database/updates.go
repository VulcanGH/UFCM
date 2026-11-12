// Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later

package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/autobrr/autobrr/internal/domain"
	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/updater"
)

// UpdateState reads the update journal without opening any runtime paths.
func (r *Repository) UpdateState(ctx context.Context) (updater.State, error) {
	var s updater.State
	var raw string
	err := r.db.QueryRowContext(ctx, `SELECT revision,auto_install,highwater,release_id,version,phase,message,envelope,failed_id FROM ufc_update_state WHERE id=1`).Scan(&s.Revision, &s.AutoInstall, &s.Highwater, &s.ID, &s.Version, &s.Phase, &s.Message, &raw, &s.FailedID)
	if err != nil {
		return s, err
	}
	err = json.Unmarshal([]byte(raw), &s.Envelope)
	return s, err
}

// SaveUpdateState uses revision and sequence guards to reject stale writers.
func (r *Repository) SaveUpdateState(ctx context.Context, s updater.State) (updater.State, error) {
	raw, err := json.Marshal(s.Envelope)
	if err != nil {
		return s, err
	}
	err = r.transaction(ctx, func(tx *sql.Tx) error {
		result, err := tx.ExecContext(ctx, `UPDATE ufc_update_state SET revision=revision+1,auto_install=$1,highwater=$2,release_id=$3,version=$4,phase=$5,message=$6,envelope=$7,failed_id=$8 WHERE id=1 AND revision=$9 AND highwater<=$2`, s.AutoInstall, s.Highwater, s.ID, s.Version, s.Phase, s.Message, string(raw), s.FailedID, s.Revision)
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
		if s.Notice != nil {
			payload, err := json.Marshal(s.Notice)
			if err != nil {
				return err
			}
			if err = queueUFCNotice(ctx, tx, "update-available:"+s.ID, string(domain.NotificationEventAppUpdateAvailable), "UFC Manager Update Available", string(payload)); err != nil {
				return err
			}
		}
		if s.ID != "" && (s.Phase == "failed" || s.Phase == "deferred") {
			return queueUFCNotice(ctx, tx, "update-attention:"+s.ID+":"+s.Phase, string(domain.NotificationEventUFCAttention), "UFC Manager Update Needs Attention", s.Message)
		}
		return nil
	})
	if err != nil {
		return s, err
	}
	s.Notice = nil
	s.Revision++
	return s, nil
}
