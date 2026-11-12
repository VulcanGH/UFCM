CREATE TABLE ufc_processing_jobs (
    id TEXT PRIMARY KEY,
    acquisition_id TEXT NOT NULL UNIQUE REFERENCES ufc_acquisitions(id),
    event_id TEXT NOT NULL,
    component TEXT NOT NULL,
    destination_root TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('queued','running','completed','needs_review')),
    message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX ufc_processing_jobs_state ON ufc_processing_jobs(state,created_at);
CREATE TABLE ufc_processing_files (
    job_id TEXT NOT NULL REFERENCES ufc_processing_jobs(id),
    source_path TEXT NOT NULL,
    destination_path TEXT NOT NULL,
    stage_path TEXT NOT NULL,
    source_size BIGINT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('queued','staging','published','verified')),
    source_sha256 TEXT NOT NULL DEFAULT '',
    destination_sha256 TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    PRIMARY KEY(job_id,source_path)
);
