/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */
import type { UFCMatchingPreview } from "./UFCMatching";

export interface UFCSearchSource { id: number; name: string; type: string }
export interface UFCSearchRequest { component: string; feed_ids: number[]; query: string }
export interface UFCSearchResponse {
 event_revision: number; settings_revision: number;
  resolutions: string[];
  event_id: string;
  component: string;
  query: string;
  sources: { source: UFCSearchSource; error?: string; more: boolean; count: number }[];
  results: { published_at?: string | null; candidate_id?: string; source: UFCSearchSource; title: string; size: number; matched: boolean; quality_check: { matched: boolean; allowed: string[]; reason: string }; assessment: string; reasons: string[]; match: UFCMatchingPreview }[];
}
