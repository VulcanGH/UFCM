/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */

import { UFCActionButton } from "./UFCActionButton";
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { APIClient } from "@api/APIClient";
import type { UFCLibrarySettings as Settings } from "@app/types/UFCLibrary";
import { compactButton } from "./UFCCompact";

function RootsForm({ settings }: { settings: Settings }) {
  const client = useQueryClient();
  const [base, setBase] = useState(settings.revision);
  const [paths, setPaths] = useState(settings.roots.filter(root => root.enabled).map(root => root.path).join("\n"));
  const save = useMutation({ mutationFn: () => APIClient.ufc.saveLibrarySettings({ revision: base, paths: paths.split("\n").map(path => path.trim()).filter(Boolean) }), onSuccess: saved => { setBase(saved.revision); void client.invalidateQueries({ queryKey: ["ufc"] }); } });
  const scan = useMutation({ mutationFn: APIClient.ufc.scanLibrary, onSuccess: () => { void client.invalidateQueries({ queryKey: ["ufc"] }); } });
  return <div className="space-y-3">
    <form className="space-y-3" onSubmit={e => { e.preventDefault(); save.mutate(); }}>
      <p className="text-sm text-gray-500 dark:text-gray-400">Choose up to eight non-overlapping folders. Scans read names and metadata only; links are skipped.</p>
      <label htmlFor="library-roots" className="block text-sm font-medium">Library folders (one absolute path per line)</label>
      <textarea id="library-roots" rows={3} value={paths} disabled={settings.running || save.isPending} onChange={e => { setPaths(e.target.value); save.reset(); }} className="block w-full rounded-md border-gray-300 bg-gray-100 font-mono text-sm dark:border-gray-700 dark:bg-gray-815" />
      <div className="flex flex-wrap gap-3"><UFCActionButton busy={save.isPending} type="submit" disabled={save.isPending || settings.running} className={compactButton}>Save Library Folders</UFCActionButton><UFCActionButton busy={scan.isPending || settings.running} type="button" disabled={scan.isPending || settings.running || !settings.roots.some(root => root.enabled)} onClick={() => scan.mutate()} className={compactButton}>{settings.running ? "Scanning…" : "Scan Library"}</UFCActionButton><Link to="/ufc/library" className={compactButton}>Open Library</Link></div>
    </form>
    {save.error && <div role="alert" className="space-y-2 text-sm"><p>Could not save folders. {save.error.message} Your input is retained.</p><button type="button" onClick={() => { setBase(settings.revision); setPaths(settings.roots.filter(root => root.enabled).map(root => root.path).join("\n")); save.reset(); }} className="underline">Load latest saved folders</button></div>}
    {save.isSuccess && <p role="status" className="text-sm">Library folders saved. Scan to update coverage.</p>}
    {scan.error && <p role="alert">Scan could not start. {scan.error.message}</p>}
    {(scan.isSuccess || save.isSuccess || settings.running) && settings.message && <p role="status" className="text-sm">{settings.message}</p>}
    <ul className="space-y-1 text-sm text-gray-500 dark:text-gray-400">{settings.roots.map(root => <li key={root.id} className="break-all">{root.path} — {root.state}{root.message ? ` · ${root.message}` : ""}</li>)}</ul>
  </div>;
}

export function UFCLibrarySettings() {
  const settings = useQuery({ queryKey: ["ufc", "library-settings"], queryFn: APIClient.ufc.librarySettings, retry: false, refetchInterval: query => query.state.data?.running ? 1000 : false });
  return <fieldset aria-label="Library settings" className="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-gray-700"><legend className="px-1 font-medium">Library</legend>{settings.isPending && <p>Loading library settings…</p>}{settings.error && <p role="alert">Library settings unavailable. {settings.error.message}</p>}{settings.data && <RootsForm settings={settings.data} />}</fieldset>;
}
