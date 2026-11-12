/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */

import { UFCActionButton } from "./UFCActionButton";
import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { APIClient } from "@api/APIClient";
import type { UFCLibraryFile, UFCLibraryFileEdit } from "@app/types/UFCLibrary";
import { componentNames, UFC_COMPONENTS } from "@utils/ufcComponents";
import { Section } from "./settings/_components";
import { compactButton } from "./UFCCompact";

function ReviewFile({ file, onClose }: { file: UFCLibraryFile; onClose: () => void }) {
  const client = useQueryClient();
  const events = useQuery({ queryKey: ["ufc", "library-events"], queryFn: APIClient.ufc.libraryEvents });
  const [eventID, setEventID] = useState(file.event_id);
  const [eventSearch, setEventSearch] = useState("");
  const sortedEvents = [...(events.data ?? [])].sort((a, b) =>
    (b.date ?? "").localeCompare(a.date ?? "") || a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" })
  );
  const searchTerms = eventSearch.toLocaleLowerCase().trim().split(/\s+/).filter(Boolean);
  const matchingEvents = sortedEvents.filter(event => searchTerms.every(term => `${event.name} ${event.date ?? ""}`.toLocaleLowerCase().includes(term)));
  const visibleEvents = sortedEvents.filter(event => event.id === eventID || matchingEvents.includes(event));
  const [component, setComponent] = useState(file.component === "unknown" || file.component === "other" ? "main_card" : file.component);
  const resolution = file.resolution || "unknown";
  const save = useMutation({ mutationFn: (mode: UFCLibraryFileEdit["mode"]) => APIClient.ufc.editLibraryFile(file.id, { revision: file.revision, mode, event_id: eventID, component, resolution }), onSuccess: () => { void client.invalidateQueries({ queryKey: ["ufc"] }); onClose(); } });
  return <div className="space-y-3 rounded-md border border-blue-200 p-3 dark:border-blue-800">
    <p className="text-sm font-medium">Review {file.path.split(/[\\/]/).pop()}</p><p className="text-sm text-gray-500 dark:text-gray-400">Manual identification is protected on future scans. This only changes the inventory record.</p>
    <label className="block text-sm">Find Event<input type="search" value={eventSearch} onChange={e => setEventSearch(e.target.value)} placeholder="Search by name, number or date" className="mt-1 block w-full rounded-md border-gray-300 bg-gray-100 dark:border-gray-700 dark:bg-gray-815" /></label>
    <label className="block text-sm">Event<select size={searchTerms.length ? 6 : undefined} value={eventID} onChange={e => setEventID(e.target.value)} className="mt-1 block w-full rounded-md border-gray-300 bg-gray-100 dark:border-gray-700 dark:bg-gray-815"><option value="">Choose an existing event</option>{visibleEvents.map(event => <option key={event.id} value={event.id}>{event.name}{event.date ? ` (${event.date})` : ""}</option>)}</select></label>
    <p role="status" className="text-xs text-gray-500 dark:text-gray-400">{searchTerms.length ? matchingEvents.length ? `${matchingEvents.length} matching event${matchingEvents.length === 1 ? "" : "s"}. Select an event above.` : "No matching events." : "Newest events first."} The selected event stays available while searching.</p>
    <label className="block text-sm">Component<select value={component} onChange={e => setComponent(e.target.value)} className="mt-1 block w-full rounded-md border-gray-300 bg-gray-100 dark:border-gray-700 dark:bg-gray-815">{UFC_COMPONENTS.map(item => <option key={item.key} value={item.key}>{item.label}</option>)}</select></label>
    {events.error && <p role="alert">Could not load events. {events.error.message}</p>}
    {save.error && <p role="alert">Could not save review. {save.error.message} Your choices are retained. If the scan changed this record, close and reopen review to use its latest revision.</p>}
    <div className="flex flex-wrap gap-3"><button type="button" disabled={save.isPending || !eventID || !file.available} onClick={() => save.mutate("confirm")} className="rounded-md border border-gray-300 px-3 py-1.5 text-sm hover:text-blue-600 disabled:opacity-50 dark:border-gray-700">Confirm identification</button><button type="button" disabled={save.isPending} onClick={() => save.mutate("ignore")} className="rounded-md border border-gray-300 px-3 py-1.5 text-sm hover:text-blue-600 dark:border-gray-700">Ignore for coverage</button><button type="button" disabled={save.isPending} onClick={() => save.mutate("auto")} className="rounded-md border border-gray-300 px-3 py-1.5 text-sm hover:text-blue-600 dark:border-gray-700">Use detected values</button><button type="button" onClick={onClose} className="text-sm text-gray-500">Close Review</button></div>
    <p className="text-xs text-gray-500 dark:text-gray-400">If the event is absent, create it through Events first. Unknown historical numbering is never treated as a verified alias.</p>
  </div>;
}

function GroupFiles({ group }: { group: string }) {
  const [offset, setOffset] = useState(0);
  const [review, setReview] = useState<UFCLibraryFile>();
  const files = useQuery({ queryKey: ["ufc", "library-files", group, offset], queryFn: () => APIClient.ufc.libraryFiles(group, offset), retry: false });
  return <div className="space-y-3 border-t border-gray-200 pt-3 dark:border-gray-700">
    {files.isPending && <p>Loading files…</p>}{files.error && <p role="alert">Files unavailable. {files.error.message}</p>}
    {files.data?.files.map(file => <div key={file.id} className="space-y-1 text-sm"><div className="flex items-start justify-between gap-3"><p title={`${file.root_path}/${file.path}`} className="min-w-0 break-all">{file.path.split(/[\\/]/).pop()}</p><button type="button" onClick={() => setReview(file)} className={compactButton}>Review</button></div><p className="text-xs text-gray-500 dark:text-gray-400">{file.component === "unknown" || file.component === "other" ? file.component : componentNames([file.component])}{file.resolution && !["unknown", "unconfirmed"].includes(file.resolution) ? ` · ${file.resolution}` : ""} · {file.state}{file.manual ? " (protected)" : ""}{!file.available ? " · unavailable/missing" : ""} · {(file.size / 1024 ** 3).toFixed(2)} GiB</p>{file.reason !== "Identified from folder/filename; resolution is a title assertion, not a media probe" && <p className="text-xs text-gray-500 dark:text-gray-400">{file.reason}</p>}</div>)}
    {review && <ReviewFile key={review.id} file={review} onClose={() => setReview(undefined)} />}
    {files.data && files.data.total > 50 && <div className="flex gap-3 text-sm"><button type="button" disabled={offset === 0} onClick={() => { setOffset(Math.max(0, offset - 50)); setReview(undefined); }} className="disabled:opacity-50">Previous files</button><span>{offset + 1}–{Math.min(offset + 50, files.data.total)} of {files.data.total}</span><button type="button" disabled={offset + 50 >= files.data.total} onClick={() => { setOffset(offset + 50); setReview(undefined); }} className="disabled:opacity-50">Next files</button></div>}
  </div>;
}

export function UFCLibrary() {
  const client = useQueryClient();
  const [clearRevision, setClearRevision] = useState<number | null>(null);
  const clear = useMutation({ mutationFn: (revision: number) => APIClient.ufc.clearLibrary(revision), onSuccess: () => { setClearRevision(null); setExpanded(undefined); setPage(0); void client.invalidateQueries({ queryKey: ["ufc"] }); } });
  const view = useQuery({ queryKey: ["ufc", "library"], queryFn: APIClient.ufc.library, retry: false, refetchInterval: query => query.state.data?.settings.running ? 1000 : false });
  const scan = useMutation({ mutationFn: APIClient.ufc.scanLibrary, onSuccess: () => { void client.invalidateQueries({ queryKey: ["ufc"] }); } });
  const [search, setSearch] = useState(""); const [filter, setFilter] = useState("all"); const [page, setPage] = useState(0); const [expanded, setExpanded] = useState<string>();
  const wasRunning = useRef(false);
  useEffect(() => { if (wasRunning.current && !view.data?.settings.running) void client.invalidateQueries({ queryKey: ["ufc"] }); wasRunning.current = view.data?.settings.running ?? false; }, [view.data?.settings.running, client]);
  const groups = (view.data?.groups ?? []).filter(group => group.name.toLowerCase().includes(search.toLowerCase()) && (filter === "all" || filter === "review" && group.review > 0 || filter === "unavailable" && group.unavailable > 0));
  const safePage = Math.min(page, Math.max(0, Math.ceil(groups.length / 50) - 1));
  return <section aria-label="UFC Library" className="text-gray-900 dark:text-gray-100"><Section title="Library" description="Files grouped by event and component." rightSide={<div className="flex flex-wrap gap-3"><UFCActionButton busy={scan.isPending || !!view.data?.settings.running} type="button" disabled={clear.isPending || scan.isPending || view.data?.settings.running || !view.data?.settings.roots.some(root => root.enabled)} onClick={() => scan.mutate()} className="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-600 hover:text-blue-600 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300">{view.data?.settings.running ? "Scanning…" : "Scan Library"}</UFCActionButton><button type="button" disabled={clear.isPending || scan.isPending || view.data?.settings.running || !view.data || view.data.groups.length === 0} onClick={() => { clear.reset(); setClearRevision(view.data?.settings.revision ?? null); }} className="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-600 hover:text-red-600 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:text-red-400 focus-visible:text-red-600 dark:focus-visible:text-red-400">Clear Library</button></div>}>
    <div className="space-y-4 sm:px-2"><div className="flex items-center justify-between gap-3"><p className="text-sm text-gray-500 dark:text-gray-400">Scan for files added outside UFC Manager.</p><Link to="/ufc/settings" className={compactButton}>Configure</Link></div>
      {view.isPending && <p>Loading library…</p>}{view.error && <p role="alert">Library unavailable. {view.error.message}</p>}{scan.error && <p role="alert">Scan could not start. {scan.error.message}</p>}
      {clearRevision !== null && <div role="alert" className="space-y-3 rounded-md border border-red-300 p-3 dark:border-red-800"><p>Clear saved library inventory and manual file identifications? Files on disk, configured folders and the event catalogue are kept. Scan again to rebuild inventory.</p><div className="flex gap-3"><button type="button" disabled={clear.isPending || scan.isPending || view.data?.settings.running} onClick={() => clear.mutate(clearRevision)} className="rounded-md bg-red-600 px-4 py-2 text-sm text-white hover:bg-red-700 disabled:opacity-50">{clear.isPending ? "Clearing…" : "Confirm Clear Library"}</button><button type="button" disabled={clear.isPending} onClick={() => setClearRevision(null)} className="rounded-md border border-gray-300 px-4 py-2 text-sm dark:border-gray-700">Cancel</button></div>{clear.error && <p>Could not clear library. {clear.error.message} If inventory changed, cancel and reopen confirmation.</p>}</div>}
      {view.data?.settings.message && (view.data.settings.running || /unavailable|incomplete|interrupted|failed/i.test(view.data.settings.message)) && <p role="status" className="text-sm">{view.data.settings.message}</p>}
      <div className="flex flex-wrap gap-3"><input aria-label="Find library event" value={search} onChange={e => { setSearch(e.target.value); setPage(0); }} placeholder="Find an event or folder" className="min-w-0 flex-1 rounded-md border-gray-300 bg-gray-100 text-sm dark:border-gray-700 dark:bg-gray-815" /><select aria-label="Library filter" value={filter} onChange={e => { setFilter(e.target.value); setPage(0); }} className="rounded-md border-gray-300 bg-gray-100 text-sm dark:border-gray-700 dark:bg-gray-815"><option value="all">All events/folders</option><option value="review">Needs Review</option><option value="unavailable">Unavailable/missing</option></select></div>
      <p className="text-sm text-gray-500 dark:text-gray-400">{groups.length} event/folder groups</p>
      {groups.some(group => !group.event_id) && <p className="text-xs text-gray-500 dark:text-gray-400">Identified events are listed first. Unidentified folders follow for review.</p>}
      <ul className="divide-y divide-gray-200 rounded-lg border border-gray-200 dark:divide-gray-700 dark:border-gray-700">{groups.slice(safePage * 50, safePage * 50 + 50).map(group => <li key={group.id} className="space-y-2 px-4 py-3"><div className="flex items-center justify-between gap-4"><div className="min-w-0 space-y-2"><h3 className="flex flex-wrap items-center gap-x-2 gap-y-1 font-medium leading-6"><span>{group.name}</span>{group.date && <span className="inline-flex items-center self-center text-xs font-normal leading-4 text-gray-500 dark:text-gray-400">({group.date})</span>}</h3>{group.review > 0 && <span className="text-xs text-amber-700 dark:text-amber-300">Needs Review</span>}{group.event_id && <div className="flex flex-wrap gap-2">{(group.wanted ?? []).map(component => <span key={component} title={`${componentNames([component])}: ${group.components[component]}`} className={`rounded px-2 py-0.5 text-xs ${group.components[component] === "present" ? "bg-green-100 text-green-800 dark:bg-green-950 dark:text-green-300" : group.components[component] === "review" ? "bg-amber-100 text-amber-800 dark:bg-amber-950 dark:text-amber-300" : "bg-gray-100 text-gray-600 dark:bg-gray-750 dark:text-gray-400"}`}>{componentNames([component])}</span>)}</div>}</div><button type="button" aria-expanded={expanded === group.id} onClick={() => setExpanded(expanded === group.id ? undefined : group.id)} className={`${compactButton} shrink-0`}>{expanded === group.id ? "Hide Files" : "Show Files"}</button></div>{expanded === group.id && <GroupFiles key={group.id} group={group.id} />}</li>)}</ul>
      {groups.length === 0 && <p>No library groups to show. Configure folders and scan, or change the filter.</p>}
      {groups.length > 50 && <div className="flex gap-3 text-sm"><button type="button" disabled={safePage === 0} onClick={() => setPage(safePage - 1)} className="disabled:opacity-50">Previous</button><span>Page {safePage + 1} of {Math.ceil(groups.length / 50)}</span><button type="button" disabled={(safePage + 1) * 50 >= groups.length} onClick={() => setPage(safePage + 1)} className="disabled:opacity-50">Next</button></div>}
      {view.data?.settings.last_scan && <p className="text-right text-xs text-gray-500 dark:text-gray-400">Last full scan: {new Date(view.data.settings.last_scan).toLocaleString()}</p>}
    </div>
  </Section></section>;
}
