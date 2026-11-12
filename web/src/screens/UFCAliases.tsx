/*
 * Copyright (c) 2026, UFC Manager contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import type { UFCEventAlias } from "@app/types/UFC";
import { aliasNamespaces } from "@utils/ufcAliases";

export function UFCAliases({ values, onChange, disabled }: { values: UFCEventAlias[]; onChange: (values: UFCEventAlias[]) => void; disabled?: boolean }) {
  const change = (index: number, patch: Partial<UFCEventAlias>) => onChange(values.map((value, i) => i === index ? { ...value, ...patch } : value));
  return <fieldset disabled={disabled} className="space-y-3">
    <p className="text-sm text-gray-500 dark:text-gray-400">Keep UFC, Fight Night, Vegas and ESPN numbers separate. Mark an alias verified only after reviewing its identity. Former numbers are not current identifiers.</p>
    {values.map((alias, index) => <div key={index} className="space-y-2 rounded-md border border-gray-200 p-3 dark:border-gray-700">
      <div className="grid gap-2 sm:grid-cols-2">
        <label className="text-sm">Alias {index + 1} kind<select value={alias.namespace} onChange={e => change(index, { namespace: e.target.value, verified: false })} className="mt-1 block w-full rounded-md border-gray-300 bg-gray-100 dark:border-gray-700 dark:bg-gray-815">{aliasNamespaces.map(([key, label]) => <option key={key} value={key}>{label}</option>)}</select></label>
        <label className="text-sm">Alias {index + 1} value<input value={alias.value} maxLength={512} onChange={e => change(index, { value: e.target.value, verified: false })} className="mt-1 block w-full rounded-md border-gray-300 bg-gray-100 dark:border-gray-700 dark:bg-gray-815" /></label>
      </div>
      <div className="flex flex-wrap items-center gap-4 text-sm">
        <label className="flex items-center gap-2"><input type="checkbox" checked={alias.current} onChange={e => change(index, { current: e.target.checked, verified: false })} />Current</label>
        <label className="flex items-center gap-2"><input type="checkbox" checked={alias.verified} onChange={e => change(index, { verified: e.target.checked })} />Verified</label>
        <button type="button" aria-label={`Remove alias ${index + 1}`} onClick={() => onChange(values.filter((_, i) => i !== index))} className="text-gray-600 hover:text-blue-600 dark:text-gray-300 dark:hover:text-blue-500">Remove</button>
      </div>
    </div>)}
    <button type="button" disabled={values.length >= 96} onClick={() => onChange([...values, { namespace: "title", value: "", current: true, verified: false, manual: true }])} className="text-sm font-medium text-gray-600 hover:text-blue-600 dark:text-gray-300 dark:hover:text-blue-500">Add alias</button>
  </fieldset>;
}
