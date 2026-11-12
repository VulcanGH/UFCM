// Copyright (c) 2026, UFC Manager contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package processing

// Job records one explicit copy request and its durable outcome.
type Job struct {
	ID               string    `json:"id"`
	AcquisitionID    string    `json:"acquisition_id"`
	EventID          string    `json:"event_id"`
	Component        string    `json:"component"`
	DestinationRoot  string    `json:"destination_root"`
	State            string    `json:"state"`
	Resolution       string    `json:"resolution"`
	Message          string    `json:"message"`
	CreatedAt        string    `json:"created_at"`
	UpdatedAt        string    `json:"updated_at"`
	CleanupRequested bool      `json:"cleanup_requested"`
	CleanupState     string    `json:"cleanup_state"`
	CleanupMessage   string    `json:"cleanup_message"`
	Files            []JobFile `json:"files"`
}

// JobFile stores the planned paths and copy evidence for one media file.
type JobFile struct {
	Source              string `json:"source"`
	Component           string `json:"component"`
	Destination         string `json:"destination"`
	Stage               string `json:"stage"`
	OriginalDestination string `json:"original_destination"`
	Size                int64  `json:"size"`
	CopiedBytes         int64  `json:"copied_bytes"`
	State               string `json:"state"`
	SourceSHA256        string `json:"source_sha256"`
	DestinationSHA256   string `json:"destination_sha256"`
	Message             string `json:"message"`
}
