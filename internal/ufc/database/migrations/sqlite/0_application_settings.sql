CREATE TABLE ufc_application_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    revision INTEGER NOT NULL CHECK (revision >= 1),
    automatic_acquisition_enabled BOOLEAN NOT NULL DEFAULT FALSE CHECK (automatic_acquisition_enabled IN (0, 1)),
    processing_enabled BOOLEAN NOT NULL DEFAULT FALSE CHECK (processing_enabled IN (0, 1))
);
INSERT INTO ufc_application_settings (id, revision, automatic_acquisition_enabled, processing_enabled)
VALUES (1, 1, FALSE, FALSE);
