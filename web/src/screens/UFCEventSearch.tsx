/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */

import { UFCActionButton } from "./UFCActionButton";
import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { APIClient } from "@api/APIClient";
import { UFCEventQueryOptions } from "@api/queries";
import { componentNames, UFC_COMPONENTS } from "@utils/ufcComponents";
import { compactButton, UFCCompactToggle } from "./UFCCompact";

export function UFCEventSearch({ id, onClose }: { id: string; onClose: () => void }) {
  const queryClient = useQueryClient();
  const library = useQuery({ queryKey: ["ufc", "library-coverage"], queryFn: APIClient.ufc.libraryCoverage, retry: false, refetchInterval: query => query.state.data?.running ? 1000 : false });
  const clients = useQuery({ queryKey: ["ufc", "download-clients"], queryFn: APIClient.ufc.downloadClients, retry: false });
  const processing = useQuery({ queryKey: ["ufc", "processing-settings"], queryFn: APIClient.ufc.processingSettings, retry: false });
  const [chosenClient, setChosenClient] = useState<number>();
  const [categoryOverride, setCategoryOverride] = useState<string | null>(null);
  const [confirmCandidate, setConfirmCandidate] = useState<string>();
  const [manualAssignment,setManualAssignment]=useState(false);
  const [manualReason,setManualReason]=useState("");
  const preferredClient = clients.data?.some(client => client.id === processing.data?.sab_client_id) ? processing.data?.sab_client_id : undefined;
  const clientID = chosenClient ?? preferredClient ?? clients.data?.[0]?.id ?? 0;
  const category = categoryOverride ?? (clientID === processing.data?.sab_client_id ? processing.data.sab_category : "");
  const send = useMutation({
    mutationFn: (candidateID: string) => APIClient.ufc.submitDownload({ candidate_id: candidateID, client_id: clientID, category, ...(manualAssignment ? {manual_reason:manualReason.trim(),event_revision:search.data?.event_revision,settings_revision:search.data?.settings_revision}: {}) }),
    onSuccess: () => { setConfirmCandidate(undefined); void queryClient.invalidateQueries(); }
  });
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => { heading.current?.focus(); }, []);
  const event = useQuery(UFCEventQueryOptions(id));
  const sources = useQuery({ queryKey: ["ufc", "search-sources"], queryFn: APIClient.ufc.searchSources, retry: false });
  const [component, setComponent] = useState("main_card");
  const coverageState = library.data?.events[id]?.[component];
  const libraryBlocked = library.isPending || library.isError || !!library.data?.configured && (library.data.running || library.data.unavailable || coverageState === "present" || coverageState === "review" || coverageState === "unavailable");
  const libraryBlockReason = library.isPending ? "Checking Library coverage before a download can be sent."
    : library.isError ? "Library coverage could not be checked."
    : library.data?.running ? "A Library scan is in progress."
    : library.data?.unavailable ? "A configured Library folder is unscanned or unavailable. Scan Library to check what you already have."
    : coverageState === "present" ? "This event component is already in your Library."
    : coverageState === "review" || coverageState === "unavailable" ? "Review this event component in Library before downloading another copy."
    : null;
  const [excludedSources, setExcludedSources] = useState<number[]>([]);
  const selected = sources.data?.filter(source => !excludedSources.includes(source.id)).map(source => source.id) ?? [];
  const [query, setQuery] = useState("");
  const [showOtherResults, setShowOtherResults] = useState(false);
  const search = useMutation({ mutationFn: () => APIClient.ufc.searchEvent(id, { component, feed_ids: selected, query }) });
  const matchingResults = search.data?.results.filter(result => result.matched) ?? [];
  const displayedResults = showOtherResults ? search.data?.results ?? [] : matchingResults;
  const inputClass = "block w-full rounded-md border-gray-300 bg-gray-100 dark:border-gray-700 dark:bg-gray-815";
  return <section aria-label="Event search" className="space-y-4 rounded-lg border border-gray-300 p-4 dark:border-gray-600">
    <div className="flex items-center justify-between gap-3"><h3 ref={heading} tabIndex={-1} className="font-semibold">Manual Search: {event.data?.values.name ?? "Event"}</h3><button type="button" onClick={onClose} className={compactButton}>Close Search</button></div>
    <p className="text-sm text-gray-500 dark:text-gray-400">Search existing configured connections. Nothing is sent until you explicitly confirm a download. One page per connection, up to 50 results each and 250 overall.</p>
    {event.data?.quality && <div className="rounded-md border border-gray-200 p-3 text-sm dark:border-gray-700"><span className="font-medium">Accepted resolutions:</span><div className="mt-2 flex flex-wrap gap-2">{event.data.quality.effective.map(value => <span key={value} className="rounded bg-blue-100 px-2 py-0.5 text-xs text-blue-800 dark:bg-blue-950 dark:text-blue-300">{value}</span>)}</div></div>}
    {libraryBlocked && libraryBlockReason && <p role="status" className="text-sm text-amber-800 dark:text-amber-300">Download is unavailable: {libraryBlockReason} <Link to="/ufc/library" className="text-gray-600 hover:text-blue-600 dark:text-gray-300 dark:hover:text-blue-500">Open Library</Link></p>}
    {library.error && <p role="alert">Library coverage unavailable. {library.error.message}</p>}
    {event.error && <p role="alert">Could not read event. {event.error.message}</p>}
    {sources.error && <p role="alert">Could not load search connections. {sources.error.message}</p>}
    {sources.isPending && <p role="status">Loading configured connections…</p>}
    {sources.data?.length === 0 && <p>No enabled Newznab/Torznab connections found. Use the existing indexer/feed settings to configure one. IRC and RSS-only sources do not support this search.</p>}
    <form className="space-y-3" onSubmit={e => { e.preventDefault(); search.mutate(); }}>
      <fieldset disabled={search.isPending} className="space-y-3">
        <div><label htmlFor="search-component" className="mb-1 block text-sm font-medium">Component</label><select id="search-component" value={component} onChange={e => { setComponent(e.target.value); search.reset(); }} className={inputClass}>{UFC_COMPONENTS.map(item => <option key={item.key} value={item.key}>{item.label}{event.data?.components?.wanted.includes(item.key) ? " (wanted)" : ""}</option>)}</select></div>
        <div><label htmlFor="event-query" className="mb-1 block text-sm font-medium">Event query (optional)</label><input id="event-query" maxLength={160} value={query} onChange={e => { setQuery(e.target.value); search.reset(); }} placeholder="Use the event name or verified UFC number" className={inputClass} /><p className="mt-1 text-xs text-gray-500 dark:text-gray-400">Leave blank for the event query. Adjust it to try another known name or numbering alias. The component is checked locally, so unlabelled results remain visible for review.</p></div>
        <fieldset className="space-y-2"><legend className="mb-1 text-sm font-medium">Existing Connections (All Selected By Default)</legend>{sources.data?.map(source => <div key={source.id} className="flex items-center gap-2"><UFCCompactToggle label={source.name} checked={selected.includes(source.id)} onChange={checked => { setExcludedSources(current => checked ? current.filter(id => id !== source.id) : [...current, source.id]); search.reset(); }} /><span className="text-xs text-gray-500 dark:text-gray-400">{source.type}</span></div>)}</fieldset>
        <div className="flex flex-wrap items-center gap-2"><UFCActionButton busy={search.isPending} type="submit" disabled={!event.data || selected.length === 0 || search.isPending} className={compactButton}>{search.isPending ? "Searching…" : "Search releases"}</UFCActionButton><Link to="/ufc/downloads" className={compactButton}>View download progress</Link></div>
      </fieldset>
    </form>
    {search.error && <p role="alert">Search could not be completed. {search.error.message}</p>}
    {send.error && <p role="alert">Could not submit download. {send.error.message}</p>}
    {send.data && <p role="status">{send.data.existing ? "Already tracked" : "Download tracked"}: {send.data.job.title} — {send.data.job.state.replaceAll("_", " ")}. {send.data.job.message}</p>}
    {search.data && <div className="space-y-3" aria-live="polite">
      <fieldset disabled={send.isPending} className="space-y-3 rounded-md border border-gray-200 p-3 dark:border-gray-700"><legend className="px-1 font-medium">Download Destination</legend>
        {clients.error && <p role="alert">Could not read configured SABnzbd clients. {clients.error.message}</p>}
        {clients.data?.length === 0 && <p>Enable a SABnzbd client in the existing download-client settings to download NZB results.</p>}
        <label className="block text-sm font-medium">Existing SABnzbd client<select value={clientID} onChange={e => { setChosenClient(Number(e.target.value)); setCategoryOverride(null); setConfirmCandidate(undefined); send.reset(); }} className={inputClass}>{!clients.data?.length && <option value={0}>No enabled SABnzbd client</option>}{clients.data?.map(client => <option key={client.id} value={client.id}>{client.name}</option>)}</select></label>
        <label className="block text-sm font-medium">Category (optional)<input value={category} maxLength={128} onChange={e => { setCategoryOverride(e.target.value); setConfirmCandidate(undefined); send.reset(); }} placeholder="Use SABnzbd default" className={inputClass} /></label>
        <p className="text-xs text-gray-500 dark:text-gray-400">SABnzbd applies the category's configured download behaviour. Selected search results expire after 15 minutes.</p>
      </fieldset>
      <p className="text-sm">Query: <strong>{search.data.query}</strong> · Component: {componentNames([search.data.component])} · Quality: {search.data.resolutions.join(", ")} · {matchingResults.length} matches · {search.data.results.length - matchingResults.length} non-matching results hidden by default</p>
      {libraryBlocked && libraryBlockReason && <p role="status" className="rounded-md border border-amber-300 p-3 text-sm text-amber-800 dark:border-amber-700 dark:text-amber-300">Sending is paused: {libraryBlockReason} <Link to="/ufc/library" className="text-gray-600 hover:text-blue-600 dark:text-gray-300 dark:hover:text-blue-500">Open Library</Link></p>}
      <UFCCompactToggle label="Show Non-Matching Results And Reasons" checked={showOtherResults} onChange={setShowOtherResults} />
      {search.data.sources.map(state => <p key={state.source.id} className={`text-sm ${state.error ? "text-amber-800 dark:text-amber-300" : "text-gray-500 dark:text-gray-400"}`}>{state.source.name}: {state.error ?? `${state.count} returned${state.more ? " — limited page; more results may be available" : ""}`}</p>)}
      {displayedResults.map((result, i) => <article key={`${result.source.id}-${i}`} className="space-y-2 rounded-lg border border-gray-200 p-3 dark:border-gray-700">
        <h4 className="break-words font-medium">{result.title}</h4>
        {result.matched && result.candidate_id && <button type="button" disabled={!clientID || send.isPending || libraryBlocked} onClick={() => { setConfirmCandidate(result.candidate_id); setManualAssignment(false); setManualReason(""); send.reset(); }} className={compactButton}>Download</button>}
        {!result.matched && result.candidate_id && <button type="button" disabled={!clientID || send.isPending || libraryBlocked || !result.quality_check.matched} onClick={()=>{setConfirmCandidate(result.candidate_id);setManualAssignment(true);setManualReason("");send.reset();}} className={compactButton}>Assign manually</button>}
        {confirmCandidate && confirmCandidate === result.candidate_id && <div role="alert" className="space-y-2 rounded-md border border-blue-200 p-3 dark:border-blue-800">{manualAssignment && <div className="space-y-2"><p className="font-medium">Manual assignment: {event.data?.values.name} / {componentNames([search.data.component])}</p><p>This overrides event/component matching for this download only. It does not teach the automatic matcher or change aliases. Resolution, library and duplicate checks still apply.</p><label className="block">Reason for override<input aria-label="Manual assignment reason" maxLength={500} value={manualReason} onChange={e=>setManualReason(e.target.value)} className={inputClass} placeholder="Explain why this release belongs to this event/component" /></label></div>}<p>Download <strong>{result.title}</strong> with {clients.data?.find(client => client.id === clientID)?.name}? Category: {category || "SABnzbd default"}.</p><div className="flex gap-3"><button type="button" disabled={send.isPending || !clientID || libraryBlocked || manualAssignment && manualReason.trim().length<8} onClick={() => send.mutate(confirmCandidate)} className="rounded-md bg-blue-600 px-3 py-2 text-sm text-white hover:bg-blue-700 disabled:opacity-50">{send.isPending ? "Submitting…" : "Confirm download"}</button><button type="button" disabled={send.isPending} onClick={() => setConfirmCandidate(undefined)} className="text-sm text-gray-600 hover:text-blue-600 dark:text-gray-300">Cancel</button></div></div>}
        {result.matched && result.source.type === "TORZNAB" && <p className="text-xs text-gray-500 dark:text-gray-400">Torrent result — cannot be sent to SABnzbd.</p>}
        {result.matched && result.source.type === "NEWZNAB" && !result.candidate_id && <p className="text-xs text-gray-500 dark:text-gray-400">No downloadable NZB selection is available for this result. Try a fresh search or check the indexer.</p>}
        <p className="text-sm text-gray-500 dark:text-gray-400">{result.source.name} · {result.size > 0 ? `${(result.size / 1024 ** 3).toFixed(2)} GiB` : "Size unknown"} · {result.match.quality.values.join(", ") || "Quality unknown"} · {publicationLabel(result.published_at)}</p>
        <p className="text-sm">Selected criteria: <strong>{result.assessment}</strong> · Event identity: {result.match.status} · Components: {result.match.components.values.length ? componentNames(result.match.components.values) : "Unknown"} ({result.match.components.status})</p>
        {result.reasons.length > 0 && <ul className="list-disc pl-5 text-sm text-amber-800 dark:text-amber-300">{result.reasons.map(reason => <li key={reason}>{reason}</li>)}</ul>}
        <details className="text-sm"><summary className="cursor-pointer">Matching evidence</summary>{result.match.candidates.map(candidate => <div key={candidate.id} className="mt-2"><p className="font-medium">{candidate.name}</p><ul className="list-disc pl-5">{[...candidate.reasons, ...candidate.conflicts].map((reason, i) => <li key={i}>{reason}</li>)}</ul></div>)}<p className="mt-2">Quality: {result.match.quality.status}. Main Card check: {result.match.main_card.status}.</p>{result.match.main_card.matched_terms.length > 0 && <p>Main Card excluded terms: {result.match.main_card.matched_terms.join(", ")}</p>}</details>
      </article>)}
      {search.data.results.length === 0 && <p>No releases returned. Check source messages above, or try another event query.</p>}
      {search.data.results.length > 0 && matchingResults.length === 0 && !showOtherResults && <p>No releases match the selected criteria. Enable “Show non-matching results and reasons” to inspect why.</p>}
    </div>}
  </section>;
}

function publicationLabel(value?: string|null): string {
 if (!value) return "Publication date unknown";
 const timestamp=Date.parse(value);
 if (!Number.isFinite(timestamp)) return "Publication date unknown";
 const hours=Math.floor((Date.now()-timestamp)/3600000);
 const age=hours<0?"future date reported":hours<1?"less than an hour old":hours<24?`${hours}h old`:`${Math.floor(hours/24)}d old`;
 return `Published ${new Date(timestamp).toLocaleString()} (${age})`;
}
