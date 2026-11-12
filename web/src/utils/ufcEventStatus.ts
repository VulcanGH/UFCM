/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */

export function eventStatusLabel(value: string): string {
  return value.replaceAll("_", " ").replace(/\b\w/g, letter => letter.toUpperCase());
}
