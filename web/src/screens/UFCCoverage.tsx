/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */
import { useQuery } from "@tanstack/react-query";
import { APIClient } from "@api/APIClient";
import { componentNames } from "@utils/ufcComponents";

export function UFCCoverage({ id }: { id: string }) {
  const coverage = useQuery({ queryKey: ["ufc", "library-coverage"], queryFn: APIClient.ufc.libraryCoverage, retry: false, refetchInterval: query => query.state.data?.running ? 1000 : false });
  if (!coverage.data?.configured) return null;
  return <div className="flex flex-wrap gap-2">{(coverage.data.wanted?.[id] ?? []).map(component => {
    const state = coverage.data.events[id]?.[component] ?? (coverage.data.unavailable ? "unavailable" : "missing");
    return <span key={component} title={`${componentNames([component])}: ${state}`} className={`rounded px-2 py-0.5 text-xs ${state === "present" ? "bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-300" : state === "review" ? "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300" : "bg-gray-100 text-gray-600 dark:bg-gray-750 dark:text-gray-400"}`}>{componentNames([component])}</span>;
  })}</div>;
}
