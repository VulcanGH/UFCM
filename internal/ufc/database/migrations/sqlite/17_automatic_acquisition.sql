CREATE TABLE ufc_automation_events (
 event_id TEXT PRIMARY KEY, admitted_at TEXT NOT NULL,
 archived_at TEXT NOT NULL DEFAULT '', reason TEXT NOT NULL DEFAULT ''
);
CREATE TABLE ufc_automation_inbox (
 id TEXT PRIMARY KEY, feed_id INTEGER NOT NULL, config_key TEXT NOT NULL,
 event_id TEXT NOT NULL, component TEXT NOT NULL,
 item_json TEXT NOT NULL, state TEXT NOT NULL DEFAULT 'pending', message TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL, next_due TEXT NOT NULL, attempts INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX ufc_automation_pending ON ufc_automation_inbox(state,next_due);
CREATE TABLE ufc_notification_outbox (
 id TEXT PRIMARY KEY, event TEXT NOT NULL, title TEXT NOT NULL, message TEXT NOT NULL,
 delivered BOOLEAN NOT NULL DEFAULT FALSE, attempts INTEGER NOT NULL DEFAULT 0,
 next_due TEXT NOT NULL, created_at TEXT NOT NULL
);
CREATE INDEX ufc_notification_pending ON ufc_notification_outbox(delivered,next_due);
