ALTER TABLE ufc_processing_files ADD COLUMN placeholder_due TEXT NOT NULL DEFAULT '';
CREATE INDEX ufc_processing_files_placeholder_due ON ufc_processing_files(placeholder_due) WHERE placeholder_due<>'';
