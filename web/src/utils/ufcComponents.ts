/*
 * Copyright (c) 2026, UFC Manager contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

export const UFC_COMPONENTS = [
  { key: "main_card", label: "Main Card" },
  { key: "prelims", label: "Prelims" },
  { key: "early_prelims", label: "Early Prelims" },
  { key: "countdown", label: "Countdown" },
  { key: "embedded", label: "Embedded" },
  { key: "weigh_ins", label: "Weigh-ins" }
] as const;

export const UFC_EVENT_TYPES = [
  { key: "numbered", label: "Numbered events" },
  { key: "fight_night", label: "Fight Nights" }
] as const;

export const componentNames = (keys: readonly string[]) => UFC_COMPONENTS.filter(item => keys.includes(item.key)).map(item => item.label).join(", ") || "None";
