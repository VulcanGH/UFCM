/*
 * Copyright (c) 2026, UFC Manager contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import type { UFCEventAlias } from "@app/types/UFC";

export const aliasNamespaces = [
  ["title", "Alternative title"], ["ufc.numbered", "UFC number"], ["ufc.fight_night", "Fight Night number"],
  ["ufc.vegas", "UFC Vegas number"], ["ufc.espn", "ESPN number"], ["ufc.espn_plus", "ESPN+ number"],
  ["ufc.abc", "ABC number"], ["ufc.freedom", "Freedom number"]
] as const;

export const orderedAliases = (aliases: UFCEventAlias[] | null | undefined) => [...(aliases ?? [])].sort((a, b) => a.namespace.localeCompare(b.namespace) || a.value.localeCompare(b.value) || Number(b.current) - Number(a.current) || Number(b.verified) - Number(a.verified));

export const describeAliases = (aliases: UFCEventAlias[] | null | undefined) => orderedAliases(aliases).map(a => `${aliasNamespaces.find(([key]) => key === a.namespace)?.[1] ?? a.namespace}: ${a.value}${a.current ? "" : " (former)"}${a.verified ? "" : " (unverified)"}`).join("; ") || "None";
