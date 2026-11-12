ALTER TABLE ufc_processing_jobs ADD COLUMN cleanup_requested BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE ufc_processing_jobs ADD COLUMN cleanup_state TEXT NOT NULL DEFAULT 'disabled';
ALTER TABLE ufc_processing_jobs ADD COLUMN cleanup_message TEXT NOT NULL DEFAULT '';
CREATE INDEX ufc_processing_jobs_cleanup ON ufc_processing_jobs(cleanup_state,updated_at);
