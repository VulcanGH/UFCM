/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { APIClient } from "@api/APIClient";
import { UFCCatalogueStatusQueryOptions, UFCSettingsQueryOptions } from "@api/queries";

export function UFCCatalogueRecovery() {
  const client = useQueryClient();
  const [confirm, setConfirm] = useState(false);
  const [acknowledged, setAcknowledged] = useState(false);
  const [resetConfirm, setResetConfirm] = useState(false);
  const [resetAcknowledged, setResetAcknowledged] = useState(false);
  const reset = useMutation({ mutationFn: () => APIClient.ufc.resetCatalogue(), onSuccess: async () => { setResetConfirm(false); setResetAcknowledged(false); await client.resetQueries({queryKey:UFCSettingsQueryOptions().queryKey}); await client.invalidateQueries({queryKey:["ufc"]}); } });
  const status = useQuery({ ...UFCCatalogueStatusQueryOptions(), refetchInterval: query => query.state.data?.running ? 1000 : false });
  const rebuild = useMutation({ mutationFn: () => APIClient.ufc.refreshCatalogue("rebuild", true), onSuccess: () => { setConfirm(false); setAcknowledged(false); void client.invalidateQueries({ queryKey: ["ufc"] }); } });
  const busy = reset.isPending || rebuild.isPending || status.isPending || status.isError || status.data?.running;
  return <section className="space-y-3 rounded-lg border border-red-200 p-3 dark:border-red-900" aria-label="Catalogue recovery">
    <h3 className="font-medium">Catalogue Recovery</h3>
    <p className="text-sm text-gray-500 dark:text-gray-400">Use only when historical metadata needs repairing. Routine updates belong in Upcoming Events and Past Events.</p>
    <div className="flex flex-wrap gap-3"><button type="button" disabled={busy} onClick={() => { setResetConfirm(false); setConfirm(true); setAcknowledged(false); rebuild.reset(); }} className="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50">Rebuild historical catalogue</button><button type="button" disabled={busy} onClick={() => { setConfirm(false); setResetConfirm(true); setResetAcknowledged(false); reset.reset(); }} className="rounded-md bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50">Reset event catalogue</button></div>
    {resetConfirm && <div role="alert" className="space-y-3 rounded-md border border-red-300 p-3 text-sm dark:border-red-800"><p>This empties Upcoming and Past Events, removes aliases, source cache, manual events, event corrections and per-event preferences, and disables automatic refresh. Manual event edits cannot be recovered from Wikipedia. Library inventory, download history, configured settings and all disk files are kept. Imported event identity links are retained for reconnection; manual-only event assignments may need review. Rebuilding the catalogue afterward can take a long time.</p><label className="flex items-start gap-2"><input type="checkbox" checked={resetAcknowledged} onChange={e => setResetAcknowledged(e.target.checked)} />I understand that catalogue metadata and manual event edits will be erased.</label><div className="flex gap-3"><button type="button" disabled={busy || !resetAcknowledged} onClick={() => reset.mutate()} className="rounded-md bg-red-600 px-4 py-2 text-white hover:bg-red-700 disabled:opacity-50">Confirm reset catalogue</button><button type="button" disabled={reset.isPending} onClick={() => setResetConfirm(false)} className="rounded-md border border-gray-300 px-4 py-2 dark:border-gray-700">Cancel</button></div></div>}
    {reset.isSuccess && <p role="status" className="text-sm">Catalogue reset. Automatic refresh is disabled. Use Upcoming Refresh for scheduled events and Past Refresh or Rebuild for history.</p>}
    {reset.error && <p role="alert">Could not reset catalogue. {reset.error.message}</p>}
    {confirm && <div role="alert" className="space-y-3 rounded-md border border-red-300 p-3 text-sm dark:border-red-800"><p>This revisits every historical event and fetches its Wikipedia article and aliases again. It can take a long time. Only use it when something has gone wrong. Event IDs, protected corrections, preferences, library links and media files are preserved.</p><label className="flex items-start gap-2"><input type="checkbox" checked={acknowledged} onChange={e => setAcknowledged(e.target.checked)} />I understand this is a lengthy recovery operation.</label><div className="flex gap-3"><button type="button" disabled={busy || !acknowledged} onClick={() => rebuild.mutate()} className="rounded-md bg-red-600 px-4 py-2 text-white hover:bg-red-700 disabled:opacity-50">Confirm rebuild</button><button type="button" disabled={rebuild.isPending} onClick={() => setConfirm(false)} className="rounded-md border border-gray-300 px-4 py-2 dark:border-gray-700">Cancel</button></div></div>}
    {status.data?.running && <p role="status" className="text-sm">A catalogue operation is running.{status.data.report.alias_total ? ` Aliases: ${status.data.report.alias_processed ?? 0}/${status.data.report.alias_total} checked.` : ""}</p>}
    {rebuild.error && <p role="alert">Could not start rebuild. {rebuild.error.message}</p>}
    {status.error && <p role="alert">Catalogue status unavailable. {status.error.message}</p>}
  </section>;
}
