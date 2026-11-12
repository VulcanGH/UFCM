CREATE TABLE ufc_acquisitions (
    id TEXT PRIMARY KEY,
    revision BIGINT NOT NULL DEFAULT 1,
    event_id TEXT NOT NULL,
    component TEXT NOT NULL,
    title TEXT NOT NULL,
    source TEXT NOT NULL,
    client_id INTEGER NOT NULL,
    client_name TEXT NOT NULL,
    category TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('submitting','uncertain','queued','downloading','paused','post_processing','missing','needs_review','completed','failed')),
    remote_id TEXT NOT NULL DEFAULT '',
    percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    message TEXT NOT NULL DEFAULT '',
    release_id BIGINT NOT NULL,
    action_status_id BIGINT NOT NULL,
    release_key TEXT NOT NULL,
    endpoint_key TEXT NOT NULL,
    storage TEXT NOT NULL DEFAULT '',
    actor TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX ufc_active_component_download ON ufc_acquisitions(event_id,component) WHERE state <> 'failed';
CREATE UNIQUE INDEX ufc_active_release_download ON ufc_acquisitions(release_key) WHERE state <> 'failed';
CREATE INDEX ufc_acquisitions_state_updated ON ufc_acquisitions(state,updated_at);
