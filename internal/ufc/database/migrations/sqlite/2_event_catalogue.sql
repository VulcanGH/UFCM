CREATE TABLE ufc_events (
    id TEXT PRIMARY KEY,
    revision BIGINT NOT NULL CHECK (revision > 0),
    event_type TEXT NOT NULL CHECK (event_type IN ('numbered', 'fight_night')),
    name TEXT NOT NULL,
    event_date TEXT,
    headliners_json TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('unknown', 'scheduled', 'postponed', 'cancelled', 'completed')),
    source_page_id BIGINT NOT NULL UNIQUE CHECK (source_page_id > 0),
    source_snapshot_json TEXT NOT NULL,
    manual_corrections_json TEXT NOT NULL,
    review_json TEXT NOT NULL
);

CREATE TABLE ufc_event_aliases (
    event_id TEXT NOT NULL REFERENCES ufc_events(id) ON DELETE CASCADE,
    namespace TEXT NOT NULL,
    value TEXT NOT NULL,
    normalized_value TEXT NOT NULL,
    is_current BOOLEAN NOT NULL CHECK (is_current IN (FALSE, TRUE)),
    verified BOOLEAN NOT NULL CHECK (verified IN (FALSE, TRUE)),
    manual BOOLEAN NOT NULL CHECK (manual IN (FALSE, TRUE)),
    PRIMARY KEY (event_id, namespace, normalized_value, is_current)
);
CREATE UNIQUE INDEX ufc_current_number_owner ON ufc_event_aliases(namespace, normalized_value)
    WHERE is_current = TRUE AND verified = TRUE AND namespace <> 'title';
CREATE INDEX ufc_events_date ON ufc_events(event_date, id);

CREATE TABLE ufc_catalogue_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    revision BIGINT NOT NULL CHECK (revision > 0),
    source TEXT NOT NULL CHECK (source = 'wikipedia.en'),
    periodic_sync_enabled BOOLEAN NOT NULL DEFAULT FALSE CHECK (periodic_sync_enabled = FALSE),
    sync_interval_seconds INTEGER NOT NULL DEFAULT 21600 CHECK (sync_interval_seconds >= 3600),
    component_templates_json TEXT NOT NULL,
    last_attempt TEXT,
    last_success TEXT,
    last_error TEXT NOT NULL DEFAULT '',
    last_report_json TEXT NOT NULL DEFAULT '{}'
);
INSERT INTO ufc_catalogue_settings(id, revision, source, component_templates_json)
VALUES (1, 1, 'wikipedia.en', '{"numbered":["main_card","prelims","early_prelims"],"fight_night":["main_card","prelims"]}');

CREATE TABLE ufc_event_component_overrides (
    event_id TEXT NOT NULL REFERENCES ufc_events(id) ON DELETE CASCADE,
    component_key TEXT NOT NULL CHECK (component_key IN ('main_card', 'prelims', 'early_prelims', 'countdown', 'embedded', 'weigh_ins')),
    enabled BOOLEAN NOT NULL CHECK (enabled IN (FALSE, TRUE)),
    PRIMARY KEY (event_id, component_key)
);
