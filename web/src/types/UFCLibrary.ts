/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */
export interface UFCLibraryRoot { id: string; path: string; enabled: boolean; state: string; message: string }
export interface UFCLibrarySettings { revision: number; roots: UFCLibraryRoot[]; running: boolean; run_id: string; last_scan: string | null; message: string }
export interface UFCLibraryGroup { wanted?: string[]; id: string; event_id: string; name: string; date: string | null; files: number; review: number; unavailable: number; extras: number; locations: number; components: Record<string, string> }
export interface UFCLibraryView { settings: UFCLibrarySettings; groups: UFCLibraryGroup[]; configured: boolean }
export interface UFCLibraryFile { id: string; revision: number; root_id: string; root_path: string; path: string; size: number; modified: string; event_id: string; component: string; resolution: string; state: string; reason: string; manual: boolean; present: boolean; available: boolean; group: string; detected_event: string; detected_component: string; detected_resolution: string; detected_state: string; detected_reason: string }
export interface UFCLibraryFileEdit { revision: number; mode: "confirm" | "ignore" | "auto"; event_id: string; component: string; resolution: string }
export interface UFCLibraryCoverage { wanted?: Record<string, string[]>; configured: boolean; running: boolean; unavailable: boolean; review: number; events: Record<string, Record<string, string>> }
