/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { APIClient } from "@api/APIClient";
import type { UFCProcessingSettings as Settings } from "@app/types/UFCProcessing";
import { UFCActionButton } from "./UFCActionButton";

const inputClass = "mt-1 block w-full rounded-md border-gray-300 bg-gray-100 text-sm dark:border-gray-700 dark:bg-gray-815";

const configuration = (settings: Settings) => JSON.stringify({ ...settings, revision: 0, auto_process_completed: false, cleanup_sab_download: false });

function Form({ settings }: { settings: Settings }) {
  const client = useQueryClient();
  const [base, setBase] = useState(settings);
  const [draft, setDraft] = useState<Settings>(settings);
  const [sources, setSources] = useState(settings.source_roots.join("\n"));
  const [extensions, setExtensions] = useState(settings.extensions.join(", "));
  const [conflict, setConflict] = useState(false);
  const [latest, setLatest] = useState<Settings>();
  const clients = useQuery({ queryKey: ["ufc", "download-clients"], queryFn: APIClient.ufc.downloadClients, retry: false });
  const library = useQuery({ queryKey: ["ufc", "library-settings"], queryFn: APIClient.ufc.librarySettings, retry: false });
  const categories = useQuery({ queryKey: ["ufc", "sab-categories", draft.sab_client_id], queryFn: () => APIClient.ufc.sabCategories(draft.sab_client_id), enabled: draft.sab_client_id > 0, retry: false });
  const save = useMutation({
    mutationKey: ["ufc", "processing-settings"],
    mutationFn: () => APIClient.ufc.saveProcessingSettings({ ...draft, revision: configuration(base) === configuration(settings) ? settings.revision : draft.revision, auto_process_completed: settings.auto_process_completed, cleanup_sab_download: settings.cleanup_sab_download, source_roots: sources.split("\n").map(value => value.trim()).filter(Boolean), extensions: extensions.split(",").map(value => value.trim()).filter(Boolean) }),
    onSuccess: saved => { setBase(saved); setDraft(saved); setSources(saved.source_roots.join("\n")); setExtensions(saved.extensions.join(", ")); setConflict(false); setLatest(undefined); void client.invalidateQueries({ queryKey: ["ufc", "processing-settings"] }); },
    onError: error => { if ("status" in error && error.status === 409) setConflict(true); }
  });
  const set = <K extends keyof Settings>(key: K, value: Settings[K]) => { setDraft(current => ({ ...current, [key]: value })); save.reset(); };
  const roots = library.data?.roots.filter(root => root.enabled) ?? [];
  return <form aria-label="Processing Settings" className="space-y-4" onSubmit={event => { event.preventDefault(); if (!conflict) save.mutate(); }}>
    <p className="text-sm text-gray-500 dark:text-gray-400">Configure copy-only processing. Existing library folders are never renamed or replaced.</p>
    <div className="grid gap-4 md:grid-cols-2">
      <label className="text-sm font-medium">Completed download folders (one absolute path per line)<textarea rows={1} value={sources} onChange={event => { setSources(event.target.value); save.reset(); }} className={inputClass} /></label>
      <label className="text-sm font-medium">Library destination<select value={draft.destination_root} onChange={event => set("destination_root", event.target.value)} className={inputClass}><option value="">Select a configured library folder</option>{roots.map(root => <option key={root.id} value={root.path}>{root.path}</option>)}</select></label>
    </div>
    {library.error && <p role="alert">Could not load library folders. {library.error.message}</p>}
    <div className="grid gap-4 md:grid-cols-2">
      <label className="text-sm font-medium">Event folder template<input value={draft.event_folder_template} onChange={event => set("event_folder_template", event.target.value)} className={inputClass} /></label>
      <label className="text-sm font-medium">Main file template<input value={draft.main_file_template} onChange={event => set("main_file_template", event.target.value)} className={inputClass} /></label>
      <label className="text-sm font-medium">Extra file template<input value={draft.extra_file_template} onChange={event => set("extra_file_template", event.target.value)} className={inputClass} /></label>
      <label className="text-sm font-medium">Component folder<input value={draft.component_folder} onChange={event => set("component_folder", event.target.value)} className={inputClass} /></label>
    </div>
    <p className="text-xs text-gray-500 dark:text-gray-400">Use only {"{{.EventTitle}}"}, {"{{.Year}}"} and {"{{.Component}}"}. Main files go in the event folder; extras go in the component folder.</p>
    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
      <label className="text-sm font-medium">Preferred conflict action<select value={draft.conflict_policy} onChange={event => set("conflict_policy", event.target.value as Settings["conflict_policy"])} className={inputClass}><option value="review">Manual Review</option><option value="skip">Skip Incoming File</option><option value="keep_both">Keep Both</option></select></label>
      <label className="text-sm font-medium">Media extensions<input value={extensions} onChange={event => { setExtensions(event.target.value); save.reset(); }} className={inputClass} /></label>
      <label className="text-sm font-medium">Concurrent copies<input type="number" min={1} max={4} value={draft.max_concurrent} onChange={event => set("max_concurrent", Number(event.target.value))} className={inputClass} /></label>
      <label className="text-sm font-medium">Maximum attempts<input type="number" min={1} max={10} value={draft.max_attempts} onChange={event => set("max_attempts", Number(event.target.value))} className={inputClass} /></label>
    </div>
    <div className="grid gap-4 md:grid-cols-2">
      <label className="text-sm font-medium">SABnzbd client<select value={draft.sab_client_id} onChange={event => { setDraft(current => ({ ...current, sab_client_id: Number(event.target.value), sab_category: "" })); save.reset(); }} className={inputClass}><option value={0}>Select a client</option>{clients.data?.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label>
      <label className="text-sm font-medium">UFC category<select value={draft.sab_category} disabled={!draft.sab_client_id || categories.isPending || !!categories.error} onChange={event => set("sab_category", event.target.value)} className={inputClass}><option value="">SABnzbd default</option>{draft.sab_category && !categories.data?.includes(draft.sab_category) && <option value={draft.sab_category}>{draft.sab_category} (saved; unavailable)</option>}{categories.data?.map(value => <option key={value} value={value}>{value}</option>)}</select></label>
    </div>
    {clients.error && <p role="alert">Could not load SAB clients.</p>}{categories.error && <p role="alert">Could not load this client’s categories. Check its connection.</p>}
    <div className="space-y-2">{save.error && !conflict && <p role="alert">Could not save processing settings. {save.error.message}</p>}{conflict && <div role="alert" className="space-y-2 rounded-md bg-amber-50 p-3 text-sm dark:amber-950"><p>Settings changed since this form loaded. Your inputs are retained.</p><button type="button" className="text-gray-600 hover:text-blue-600" onClick={() => { void APIClient.ufc.processingSettings().then(setLatest); }}>Review latest settings</button>{latest && <button type="button" className="ml-3 text-gray-600 hover:text-blue-600" onClick={() => { setDraft(current => ({ ...current, revision: latest.revision })); setConflict(false); setLatest(undefined); save.reset(); }}>Keep my inputs against revision {latest.revision}</button>}</div>}{save.isSuccess && <p role="status">Processing settings saved.</p>}<UFCActionButton busy={save.isPending} type="submit" disabled={save.isPending || conflict || !library.data} className="rounded-md border border-gray-300 px-3 py-2 text-sm font-medium text-gray-600 hover:text-blue-600 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300">Save processing settings</UFCActionButton></div>
  </form>;
}

export function UFCProcessingSettings() {
  const settings = useQuery({ queryKey: ["ufc", "processing-settings"], queryFn: APIClient.ufc.processingSettings, retry: false });
  return <fieldset aria-label="Processing Settings" className="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-gray-700"><legend className="px-1 font-medium">Processing</legend>{settings.isPending && <p>Loading processing settings…</p>}{settings.error && <p role="alert">Processing settings unavailable. {settings.error.message}</p>}{settings.data && <Form settings={settings.data} />}</fieldset>;
}
