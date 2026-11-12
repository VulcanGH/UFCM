/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */

import { Switch } from "@headlessui/react";

export const compactButton = "rounded-md border border-gray-300 px-2.5 py-1 text-xs font-medium leading-5 text-gray-600 hover:text-blue-600 focus-visible:text-blue-600 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:text-blue-500";
export const compactDangerButton = "rounded-md border border-gray-300 px-2.5 py-1 text-xs font-medium leading-5 text-gray-600 hover:text-red-600 focus-visible:text-red-600 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:text-red-400";

export function UFCCompactToggle({ label, checked, onChange, disabled = false }: { label: string; checked: boolean; onChange: (value: boolean) => void; disabled?: boolean }) {
  return <div className="flex items-center gap-2 text-sm"><Switch checked={checked} onChange={onChange} disabled={disabled} aria-label={label} className={`relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors disabled:opacity-50 ${checked ? "bg-blue-600" : "bg-gray-300 dark:bg-gray-700"}`}><span className={`inline-block h-3.5 w-3.5 rounded-full bg-white transition-transform ${checked ? "translate-x-4" : "translate-x-0.5"}`} /></Switch><span>{label}</span></div>;
}
