CREATE TABLE ufc_application_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    revision INTEGER NOT NULL CHECK (revision >= 1),
    automatic_acquisition_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    processing_enabled BOOLEAN NOT NULL DEFAULT FALSE
);
INSERT INTO ufc_application_settings (id, revision, automatic_acquisition_enabled, processing_enabled)
VALUES (1, 1, FALSE, FALSE);
