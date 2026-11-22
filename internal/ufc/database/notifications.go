package database

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/autobrr/autobrr/internal/ufc/catalogue"
	"github.com/autobrr/autobrr/internal/ufc/notifications"
)

func (r *Repository) InitNotificationSettings(ctx context.Context, settings notifications.Settings, webhook string) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, "INSERT INTO ufc_notification_settings(id,revision,webhook,settings_json) VALUES(1,1,$1,$2) ON CONFLICT(id) DO NOTHING", webhook, string(raw))
	return err
}

func (r *Repository) NotificationSettings(ctx context.Context) (notifications.Settings, string, error) {
	var settings notifications.Settings
	var revision int64
	var webhook, raw string
	err := r.db.QueryRowContext(ctx, "SELECT revision,webhook,settings_json FROM ufc_notification_settings WHERE id=1").Scan(&revision, &webhook, &raw)
	if err == nil {
		err = json.Unmarshal([]byte(raw), &settings)
	}
	settings.Revision = revision
	return settings, webhook, err
}

func (r *Repository) SaveNotificationSettings(ctx context.Context, settings notifications.Settings, webhook string) (notifications.Settings, error) {
	raw, err := json.Marshal(settings)
	if err != nil {
		return settings, err
	}
	result, err := r.db.ExecContext(ctx, "UPDATE ufc_notification_settings SET revision=revision+1,webhook=$1,settings_json=$2 WHERE id=1 AND revision=$3", webhook, string(raw), settings.Revision)
	if err != nil {
		return settings, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return settings, err
	}
	if n != 1 {
		return settings, catalogue.ErrRevisionConflict
	}
	settings.Revision++
	return settings, nil
}

func (r *Repository) RecordNotificationDelivery(ctx context.Context, delivery notifications.Delivery) error {
	return r.transaction(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "INSERT INTO ufc_notification_deliveries(id,event,outcome,reason,created_at) VALUES($1,$2,$3,$4,$5)", delivery.ID, delivery.Event, delivery.Outcome, delivery.Reason, delivery.Time)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM ufc_notification_deliveries WHERE id NOT IN (SELECT id FROM ufc_notification_deliveries ORDER BY created_at DESC,id DESC LIMIT 100)")
		return err
	})
}

func (r *Repository) NotificationDeliveries(ctx context.Context) ([]notifications.Delivery, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id,event,outcome,reason,created_at FROM ufc_notification_deliveries ORDER BY created_at DESC,id DESC LIMIT 30")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	history := []notifications.Delivery{}
	for rows.Next() {
		var d notifications.Delivery
		if err := rows.Scan(&d.ID, &d.Event, &d.Outcome, &d.Reason, &d.Time); err != nil {
			return nil, err
		}
		history = append(history, d)
	}
	return history, rows.Err()
}
