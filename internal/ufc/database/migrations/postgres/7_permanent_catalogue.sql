CREATE TABLE ufc_event_workspace (
    event_id TEXT PRIMARY KEY REFERENCES ufc_events(id)
);
INSERT INTO ufc_event_workspace(event_id) SELECT id FROM ufc_events;
CREATE TABLE ufc_catalogue_source_list (
    id INTEGER PRIMARY KEY CHECK(id=1),
    list_revision BIGINT NOT NULL,
    retrieved_at TEXT NOT NULL,
    rows_json TEXT NOT NULL
);
