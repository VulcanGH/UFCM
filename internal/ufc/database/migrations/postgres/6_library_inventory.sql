CREATE TABLE ufc_library_settings (
    id INTEGER PRIMARY KEY CHECK (id=1), revision BIGINT NOT NULL DEFAULT 1,
    running BOOLEAN NOT NULL DEFAULT FALSE, run_id TEXT NOT NULL DEFAULT '',
    last_scan TEXT, message TEXT NOT NULL DEFAULT ''
);
INSERT INTO ufc_library_settings(id) VALUES(1);
CREATE TABLE ufc_library_roots (
    id TEXT PRIMARY KEY, path TEXT NOT NULL, enabled BOOLEAN NOT NULL,
    state TEXT NOT NULL, message TEXT NOT NULL DEFAULT ''
);
CREATE TABLE ufc_library_files (
    id TEXT PRIMARY KEY, revision BIGINT NOT NULL DEFAULT 1,
    root_id TEXT NOT NULL REFERENCES ufc_library_roots(id), relative_path TEXT NOT NULL,
    size BIGINT NOT NULL, modified TEXT NOT NULL,
    event_id TEXT NOT NULL DEFAULT '', component TEXT NOT NULL, resolution TEXT NOT NULL,
    state TEXT NOT NULL, reason TEXT NOT NULL, manual BOOLEAN NOT NULL DEFAULT FALSE,
    present BOOLEAN NOT NULL DEFAULT TRUE, group_key TEXT NOT NULL, last_seen TEXT NOT NULL,
    detected_event TEXT NOT NULL, detected_component TEXT NOT NULL, detected_resolution TEXT NOT NULL,
    detected_state TEXT NOT NULL, detected_reason TEXT NOT NULL, actor TEXT NOT NULL DEFAULT ''
);
CREATE INDEX ufc_library_group ON ufc_library_files(group_key);
CREATE INDEX ufc_library_event_component ON ufc_library_files(event_id,component);
