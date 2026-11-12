ALTER TABLE ufc_catalogue_settings ADD COLUMN main_card_terms_json TEXT NOT NULL DEFAULT 'null';
ALTER TABLE ufc_acquisitions ADD COLUMN decision_json TEXT NOT NULL DEFAULT '{}';
CREATE TABLE ufc_acquisition_recovery (
    id TEXT PRIMARY KEY,
    acquisition_id TEXT NOT NULL,
    actor TEXT NOT NULL,
    previous_remote_id TEXT NOT NULL,
    remote_id TEXT NOT NULL,
    previous_revision BIGINT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX ufc_recovery_acquisition ON ufc_acquisition_recovery(acquisition_id,created_at);
CREATE UNIQUE INDEX ufc_remote_job_binding ON ufc_acquisitions(endpoint_key,remote_id) WHERE remote_id <> '';
