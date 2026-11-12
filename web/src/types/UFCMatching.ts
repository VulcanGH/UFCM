/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */

export interface UFCMatchingPreview {
  version: string;
  title: string;
  normalized: string;
  status: string;
  identifiers: { namespace: string; value: string }[];
  candidates: { id: string; name: string; date: string | null; reasons: string[]; conflicts: string[] }[];
  components: { status: string; values: string[] };
  quality: { status: string; values: string[] };
  main_card: { status: string; reason: string; matched_terms: string[]; groups: { name: string; terms: string[] }[] };
  notes: string[];
}
