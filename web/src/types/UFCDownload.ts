/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */
export interface UFCDownloadClient { id: number; name: string }
export interface UFCDownload {
 decision?: { manual: boolean; reason?: string; actor: string; at: string; automatic: import("./UFCMatching").UFCMatchingPreview };
  id: string; event_id: string; component: string; title: string; source: string;
  client_id: number; client_name: string; category: string; state: string;
  remote_id: string; percent: number; message: string; release_id: number;
  revision: number; created_at: string; updated_at: string;
}
export interface UFCDownloadRequest { candidate_id: string; client_id: number; category: string; manual_reason?: string; event_revision?: number; settings_revision?: number }
export interface UFCDownloadAccepted { job: UFCDownload; existing: boolean }
