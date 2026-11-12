ALTER TABLE ufc_processing_jobs ADD COLUMN resolution TEXT NOT NULL DEFAULT '';
ALTER TABLE ufc_processing_files ADD COLUMN original_destination TEXT NOT NULL DEFAULT '';
ALTER TABLE ufc_processing_files ADD COLUMN copied_bytes BIGINT NOT NULL DEFAULT 0;
