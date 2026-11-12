ALTER TABLE ufc_catalogue_settings ADD COLUMN resolutions_json TEXT NOT NULL DEFAULT '["1080p"]';
CREATE TABLE ufc_event_quality_overrides (
    event_id TEXT PRIMARY KEY REFERENCES ufc_events(id) ON DELETE CASCADE,
    resolutions_json TEXT NOT NULL
);
