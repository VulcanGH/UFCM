ALTER TABLE ufc_events ALTER COLUMN source_page_id DROP NOT NULL;
ALTER TABLE ufc_catalogue_settings DROP CONSTRAINT ufc_catalogue_settings_periodic_sync_enabled_check;
ALTER TABLE ufc_catalogue_settings ADD COLUMN next_due TEXT;
