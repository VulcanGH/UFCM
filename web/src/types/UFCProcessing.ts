/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */

export interface UFCProcessingSettings {
  revision: number;
  source_roots: string[];
  destination_root: string;
  event_folder_template: string;
  main_file_template: string;
  extra_file_template: string;
  component_folder: string;
  conflict_policy: "review" | "skip" | "keep_both";
  extensions: string[];
  max_concurrent: number;
  max_attempts: number;
  sab_client_id: number;
  sab_category: string;
  auto_process_completed: boolean;
  cleanup_sab_download: boolean;
}

export interface UFCProcessingPreview {
  acquisition_id: string;
  event_id: string;
  event_title: string;
  destination_root: string;
  status: "ready" | "needs_review";
  reasons: string[];
  files: { source: string; destination: string; component: string; size: number; state: "planned" | "review"; reasons: string[] }[];
}

export interface UFCProcessingJob {
  id: string;
  acquisition_id: string;
  event_id: string;
  component: string;
  destination_root: string;
  state: "queued" | "running" | "completed" | "needs_review";
  resolution: "" | "skipped" | "cancelled";
  message: string;
  created_at: string;
  updated_at: string;
  cleanup_requested: boolean;
  cleanup_state: string;
  cleanup_message: string;
  files: { source: string; component: string; destination: string; original_destination: string; stage: string; size: number; copied_bytes: number; state: string; source_sha256: string; destination_sha256: string; message: string }[];
}

export interface UFCProcessingReview {
  retry: boolean;
  skip: boolean;
  cancel: boolean;
  keep_both_destination: string;
  preferred: "review" | "skip" | "keep_both";
  reason: string;
}
