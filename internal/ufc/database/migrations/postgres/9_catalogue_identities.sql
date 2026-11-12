CREATE TABLE ufc_event_identities (
    event_id TEXT PRIMARY KEY,
    source_page_id BIGINT NOT NULL UNIQUE,
    last_revision BIGINT NOT NULL
);
INSERT INTO ufc_event_identities(event_id,source_page_id,last_revision) SELECT id,source_page_id,revision FROM ufc_events WHERE source_page_id IS NOT NULL;
