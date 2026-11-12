/*
 * Copyright (c) 2026, UFC Manager contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

export interface UFCStatus {
  product_slug: string;
  product_name: string;
  schema_ready: boolean;
  settings_revision: number;
  automatic_acquisition_enabled: boolean;
  processing_enabled: boolean;
}

export interface UFCEventAlias {
  namespace: string;
  value: string;
  current: boolean;
  verified: boolean;
  manual?: boolean;
}

export interface UFCEventSummary {
  id: string;
  name: string;
  date: string | null;
  aliases: UFCEventAlias[] | null;
  needs_review: boolean;
  review: string[] | null;
  source_url: string;
}

export interface UFCEventList {
  upcoming: UFCEventSummary[];
  recent_completed: UFCEventSummary[];
}

export type UFCEventStatus = "unknown" | "scheduled" | "postponed" | "cancelled" | "completed";
export type UFCEditableField = "name" | "type" | "date" | "headliners" | "status";
export type UFCEventType = "numbered" | "fight_night";
export type UFCComponentKey = "main_card" | "prelims" | "early_prelims" | "countdown" | "embedded" | "weigh_ins";
export type UFCComponentChoice = "inherit" | "on" | "off";
export type UFCComponentTemplates = Record<UFCEventType, UFCComponentKey[]>;

export interface UFCComponentPreferences {
  defaults: UFCComponentKey[];
  overrides: Partial<Record<UFCComponentKey, boolean>>;
  wanted: UFCComponentKey[];
}

export interface UFCSettings {
 main_card_terms?: string[] | null;
  resolutions?: string[];
  revision: number;
  templates: UFCComponentTemplates;
  event_counts: Record<UFCEventType, number>;
  periodic_sync_enabled?: boolean;
  sync_interval_seconds?: number;
  next_due?: string | null;
}

export interface UFCSettingsEdit {
 main_card_terms?: string[] | null;
  resolutions?: string[];
  revision: number;
  templates: UFCComponentTemplates;
  periodic_sync_enabled?: boolean;
  sync_interval_seconds?: number;
}

export interface UFCEventValues {
  name: string;
  type: "numbered" | "fight_night";
  date: string | null;
  headliners: string[] | null;
  status: UFCEventStatus;
}

export interface UFCEvent {
  quality?: { defaults: string[]; override: string[] | null; effective: string[] };
  id: string;
  revision: number;
  values: UFCEventValues;
  source: UFCEventValues & { page_id: number; list_revision: number; article_revision: number; retrieved_at: string; aliases?: UFCEventAlias[] };
  corrections: Partial<Record<UFCEditableField, { protected: boolean; value: string | null; names?: string[]; actor: string; at: string }>> & { aliases?: { protected: boolean; values: UFCEventAlias[]; actor: string; at: string } };
  aliases: UFCEventAlias[] | null;
  review: string[] | null;
  components?: UFCComponentPreferences;
}

export interface UFCFieldEdit {
  operation: "set" | "reset";
  value?: string | null;
  names?: string[];
}

export interface UFCAliasEdit { operation: "set" | "reset"; values?: UFCEventAlias[] }
export type UFCQualityEdit = { mode: "inherit" | "custom"; resolutions?: string[] };
export type UFCEventEdit = { revision: number; quality?: UFCQualityEdit; components?: Partial<Record<UFCComponentKey, UFCComponentChoice>>; aliases?: UFCAliasEdit } & Partial<Record<UFCEditableField, UFCFieldEdit>>;
export interface UFCEventCreate { values: UFCEventValues; aliases: UFCEventAlias[] }

export interface UFCCatalogueStatus {
  running: boolean;
  interrupted: boolean;
  last_success: string | null;
  last_error: string;
  periodic_sync_enabled?: boolean;
  report: { alias_total?: number; alias_processed?: number; diagnostics: { name: string; reason: string }[] | null; diagnostics_omitted: number };
}
