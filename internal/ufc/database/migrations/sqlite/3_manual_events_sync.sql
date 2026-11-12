CREATE TABLE ufc_events_next (
    id TEXT PRIMARY KEY,
    revision BIGINT NOT NULL CHECK (revision > 0),
    event_type TEXT NOT NULL CHECK (event_type IN ('numbered', 'fight_night')),
    name TEXT NOT NULL,
    event_date TEXT,
    headliners_json TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('unknown', 'scheduled', 'postponed', 'cancelled', 'completed')),
    source_page_id BIGINT UNIQUE CHECK (source_page_id > 0),
    source_snapshot_json TEXT NOT NULL,
    manual_corrections_json TEXT NOT NULL,
    review_json TEXT NOT NULL
);
INSERT INTO ufc_events_next SELECT * FROM ufc_events;

CREATE TABLE ufc_event_aliases_next (
    event_id TEXT NOT NULL REFERENCES ufc_events_next(id) ON DELETE CASCADE,
    namespace TEXT NOT NULL,
    value TEXT NOT NULL,
    normalized_value TEXT NOT NULL,
    is_current BOOLEAN NOT NULL CHECK (is_current IN (FALSE, TRUE)),
    verified BOOLEAN NOT NULL CHECK (verified IN (FALSE, TRUE)),
    manual BOOLEAN NOT NULL CHECK (manual IN (FALSE, TRUE)),
    PRIMARY KEY (event_id, namespace, normalized_value, is_current)
);
INSERT INTO ufc_event_aliases_next SELECT * FROM ufc_event_aliases;
CREATE TABLE ufc_event_component_overrides_next (
    event_id TEXT NOT NULL REFERENCES ufc_events_next(id) ON DELETE CASCADE,
    component_key TEXT NOT NULL CHECK (component_key IN ('main_card', 'prelims', 'early_prelims', 'countdown', 'embedded', 'weigh_ins')),
    enabled BOOLEAN NOT NULL CHECK (enabled IN (FALSE, TRUE)),
    PRIMARY KEY (event_id, component_key)
);
INSERT INTO ufc_event_component_overrides_next SELECT * FROM ufc_event_component_overrides;
DROP TABLE ufc_event_aliases;
DROP TABLE ufc_event_component_overrides;
DROP TABLE ufc_events;
ALTER TABLE ufc_events_next RENAME TO ufc_events;
ALTER TABLE ufc_event_aliases_next RENAME TO ufc_event_aliases;
ALTER TABLE ufc_event_component_overrides_next RENAME TO ufc_event_component_overrides;
CREATE INDEX ufc_events_date ON ufc_events(event_date, id);
CREATE UNIQUE INDEX ufc_current_number_owner ON ufc_event_aliases(namespace, normalized_value)
    WHERE is_current = TRUE AND verified = TRUE AND namespace <> 'title';

CREATE TABLE ufc_catalogue_settings_next (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    revision BIGINT NOT NULL CHECK (revision > 0),
    source TEXT NOT NULL CHECK (source = 'wikipedia.en'),
    periodic_sync_enabled BOOLEAN NOT NULL DEFAULT FALSE CHECK (periodic_sync_enabled IN (FALSE, TRUE)),
    sync_interval_seconds INTEGER NOT NULL DEFAULT 21600 CHECK (sync_interval_seconds >= 3600),
    component_templates_json TEXT NOT NULL,
    last_attempt TEXT,
    last_success TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    last_report_json TEXT NOT NULL DEFAULT '{}',
    next_due TEXT
);
INSERT INTO ufc_catalogue_settings_next SELECT *, NULL FROM ufc_catalogue_settings;
DROP TABLE ufc_catalogue_settings;
ALTER TABLE ufc_catalogue_settings_next RENAME TO ufc_catalogue_settings;
