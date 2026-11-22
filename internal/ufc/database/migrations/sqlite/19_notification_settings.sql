CREATE TABLE ufc_notification_settings (
 id INTEGER PRIMARY KEY CHECK (id = 1),
 revision INTEGER NOT NULL,
 webhook TEXT NOT NULL,
 settings_json TEXT NOT NULL
);
CREATE TABLE ufc_notification_deliveries (
 id TEXT PRIMARY KEY,
 event TEXT NOT NULL,
 outcome TEXT NOT NULL,
 reason TEXT NOT NULL,
 created_at TEXT NOT NULL
);
UPDATE ufc_notification_outbox SET delivered=TRUE WHERE delivered=FALSE AND event='UFC_ATTENTION';
