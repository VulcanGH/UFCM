ALTER TABLE ufc_acquisitions ADD COLUMN processing_auto_pending BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX ufc_acquisitions_processing_pending ON ufc_acquisitions(processing_auto_pending,updated_at);
