/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */

import { UFCActionButton } from "./UFCActionButton";
import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { APIClient } from "@api/APIClient";
import { componentNames } from "@utils/ufcComponents";
import { Section } from "./settings/_components";

export function UFCMatching() {
  const [title, setTitle] = useState("");
  const preview = useMutation({ mutationFn: APIClient.ufc.previewMatch });
  const result = preview.data;
  return <section aria-label="UFC Matching" className="text-gray-900 dark:text-gray-100">
    <Section title="Filename Match Testing" description="Preview how a release title matches your saved events. No downloads or catalogue changes.">
      <div className="space-y-4 sm:px-2">
        <form className="space-y-3" onSubmit={e => { e.preventDefault(); if (title.trim()) preview.mutate(title.trim()); }}>
          <label htmlFor="release-title" className="block text-sm font-medium">Release title</label>
          <input id="release-title" value={title} maxLength={1024} required disabled={preview.isPending} onChange={e => { setTitle(e.target.value); preview.reset(); }} placeholder="UFC.300.Early.Prelims.1080p.WEB-DL.H264" className="block w-full rounded-md border-gray-300 bg-gray-100 dark:border-gray-700 dark:bg-gray-815" />
          <UFCActionButton busy={preview.isPending} type="submit" disabled={!title.trim() || preview.isPending} className="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-600 hover:text-blue-600 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:text-blue-500">{preview.isPending ? "Matching…" : "Preview match"}</UFCActionButton>
        </form>
        {preview.error && <p role="alert">Could not preview title. {preview.error.message}</p>}
        {result && <div className="space-y-4" aria-live="polite">
          <div className={`space-y-2 rounded-lg border p-4 ${result.main_card.status === "excluded" ? "border-red-300 text-red-800 dark:border-red-800 dark:text-red-300" : "border-gray-200 dark:border-gray-700"}`}>
            <h3 className="font-semibold">Main Card check: {result.main_card.status}</h3>
            <p className="text-sm">{result.main_card.reason}</p>
            {result.main_card.matched_terms.length > 0 && <p className="text-sm">Matched terms: {result.main_card.matched_terms.join(", ")}</p>}
            <details className="text-sm"><summary className="cursor-pointer">Main Card exclusion terms</summary><p className="mt-2">Plain words and phrases, matched without regex. Case, dots, hyphens and underscores are normalised. These exclusions apply only to Main Card.</p>{result.main_card.groups.map(group => <p key={group.name} className="mt-2"><strong>{group.name}:</strong> {group.terms.join(", ")}</p>)}</details>
          </div>
          <div className="rounded-lg border border-gray-200 p-4 dark:border-gray-700">
            <h3 className="font-semibold">Event identity: {result.status}</h3>
            <p className="mt-1 break-words text-sm text-gray-500 dark:text-gray-400">{result.title}</p>
            <p className="mt-2 text-sm">Identifiers: {result.identifiers.map(id => `${id.namespace}: ${id.value}`).join(", ") || "None recognised"}</p>
          </div>
          {result.candidates.map(candidate => <div key={candidate.id} className="space-y-2 rounded-lg border border-gray-200 p-4 dark:border-gray-700">
            <h4 className="font-medium">{candidate.name} <span className="text-sm font-normal text-gray-500 dark:text-gray-400">({candidate.date ?? "Date unknown"})</span></h4>
            <ul className="list-disc space-y-1 pl-5 text-sm">{candidate.reasons.map((reason, i) => <li key={i}>{reason}</li>)}</ul>
            {candidate.conflicts.length > 0 && <div className="text-sm text-amber-800 dark:text-amber-300"><p className="font-medium">Needs Review</p><ul className="list-disc space-y-1 pl-5">{candidate.conflicts.map((reason, i) => <li key={i}>{reason}</li>)}</ul></div>}
            <p className="break-all text-xs text-gray-500 dark:text-gray-400">Local event ID: {candidate.id}</p>
          </div>)}
          {result.candidates.length === 0 && <p>No candidate found in your saved catalogue. Check the title and known event aliases.</p>}
          <dl className="divide-y divide-gray-200 rounded-lg border border-gray-200 dark:divide-gray-700 dark:border-gray-700">
            <div className="space-y-1 p-4"><dt className="font-medium">Components — {result.components.status}</dt><dd>{result.components.values.length ? componentNames(result.components.values) : "Unknown"}</dd></div>
            <div className="space-y-1 p-4"><dt className="font-medium">Quality — {result.quality.status}</dt><dd>{result.quality.values.join(", ") || "Unknown"}</dd></div>
          </dl>
          <ul className="list-disc space-y-1 pl-5 text-sm text-gray-500 dark:text-gray-400">{result.notes.map(note => <li key={note}>{note}</li>)}</ul>
          <details className="text-sm text-gray-500 dark:text-gray-400"><summary className="cursor-pointer">Matching details</summary><p className="mt-2 break-words">Normalised: {result.normalized}</p><p>Rules: {result.version}</p></details>
        </div>}
      </div>
    </Section>
  </section>;
}
