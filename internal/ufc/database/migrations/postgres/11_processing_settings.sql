CREATE TABLE ufc_processing_settings (
    id INTEGER PRIMARY KEY CHECK (id=1),
    revision BIGINT NOT NULL DEFAULT 1,
    settings_json TEXT NOT NULL
);
INSERT INTO ufc_processing_settings(id,settings_json) VALUES(1,'{"source_roots":[],"destination_root":"","event_folder_template":"{{.EventTitle}} ({{.Year}})","main_file_template":"{{.EventTitle}} ({{.Year}})","extra_file_template":"{{.EventTitle}} ({{.Year}}) - {{.Component}}","component_folder":"Featurettes","conflict_policy":"review","extensions":[".mkv",".mp4",".avi"],"max_concurrent":1,"max_attempts":3,"sab_client_id":0,"sab_category":"","cleanup_sab_download":false}');
