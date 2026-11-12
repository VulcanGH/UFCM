/*
 * Copyright (c) 2026, UFC Manager contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { UFCActionButton } from "./UFCActionButton";
import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { APIClient } from "@api/APIClient";
import { UFCCatalogueStatusQueryOptions, UFCEventsQueryOptions } from "@api/queries";
import type { UFCEventAlias, UFCEventSummary } from "@app/types/UFC";
import { Section } from "./settings/_components";
import { UFCEventEditor } from "./UFCEventEditor";
import { UFCNewEvent } from "./UFCNewEvent";
import { UFCEventSearch } from "./UFCEventSearch";
import { UFCCoverage } from "./UFCCoverage";
import { compactButton } from "./UFCCompact";

function aliasLabel(alias: UFCEventAlias): string {
  const prefix: Record<string, string> = {
    "ufc.numbered": "UFC", "ufc.fight_night": "UFC Fight Night", "ufc.vegas": "UFC Vegas",
    "ufc.espn": "UFC on ESPN", "ufc.espn_plus": "UFC on ESPN+"
  };
  const value = prefix[alias.namespace] ? `${prefix[alias.namespace]} ${alias.value}` : alias.value;
  return `${value}${!alias.current ? " (former)" : !alias.verified ? " (unverified)" : ""}`;
}

function EventGroup({ title, events, empty, onEdit, onSearch }: { title: string; events: UFCEventSummary[]; empty: string; onEdit: (id: string) => void; onSearch: (id: string) => void }) {
  return (
    <section aria-label={title} className="space-y-3">
      {events.length === 0 ? <p className="text-gray-600 dark:text-gray-400">{empty}</p> : (
        <ul className="divide-y divide-gray-200 rounded-lg border border-gray-200 dark:divide-gray-700 dark:border-gray-700">
          {events.map(event => {
            const aliases = [...new Set((event.aliases ?? []).filter(alias => alias.namespace !== "title" || alias.value !== event.name).map(aliasLabel))];
            return (
              <li key={event.id} className="flex items-center justify-between gap-4 px-4 py-3">
                <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <h4 className="font-medium leading-6">{event.source_url ? <a href={event.source_url} target="_blank" rel="noreferrer" title="Open Wikipedia Source" className="hover:underline focus-visible:underline">{event.name}</a> : <span>{event.name}</span>}</h4>
                  {!event.source_url && <span className="text-xs text-gray-500 dark:text-gray-400">Local</span>}
                  <span className="inline-flex items-center self-center text-xs leading-4 text-gray-500 dark:text-gray-400">({event.date ? <time dateTime={event.date}>{event.date}</time> : "Date to be announced"})</span>
                  {event.needs_review && <span title={(event.review ?? []).join(", ").replaceAll("_", " ")} className="rounded bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-900 dark:bg-amber-900 dark:text-amber-100">Needs Review</span>}
                </div>
                <p className="text-xs leading-tight text-gray-500 dark:text-gray-400">Aliases: {aliases.length ? aliases.join(" · ") : "None recorded"}</p>
                <div className="pt-2"><UFCCoverage id={event.id} /></div>
                </div>
                <div className="flex shrink-0 flex-wrap justify-end gap-1.5">
                  <button type="button" onClick={() => onEdit(event.id)} aria-label={`Edit ${event.name}`} className={compactButton}>Edit</button>
                  <button type="button" onClick={() => onSearch(event.id)} aria-label={`Manual Search ${event.name}`} className={compactButton}>Manual Search</button>
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

export function UFCPastEvents() { return <UFCEvents past />; }

export function UFCEvents({ past = false }: { past?: boolean } = {}) {
  const queryClient = useQueryClient();
  const events = useQuery({ ...UFCEventsQueryOptions(), enabled: !past });
  const showAll = past;
  const [loadAll, setLoadAll] = useState(false);
  const [catalogueSearch, setCatalogueSearch] = useState("");
  const catalogue = useQuery({ queryKey: ["ufc", "events", "past", catalogueSearch, loadAll], queryFn: () => APIClient.ufc.catalogueEvents(catalogueSearch, 0, loadAll ? 2500 : 20), enabled: past, retry: false });
  const [editingID, setEditingID] = useState<string>();
  const [searchID, setSearchID] = useState<string>();
  const [creating, setCreating] = useState(false);
  const [confirmClear, setConfirmClear] = useState(false);
  const lastSuccess = useRef<string | null | undefined>(undefined);
  const refreshEvents = () => Promise.all([
    queryClient.invalidateQueries({ queryKey: UFCEventsQueryOptions().queryKey }),
    queryClient.invalidateQueries({ queryKey: ["ufc", "event"] })
  ]);
  const [awaitingStatus, setAwaitingStatus] = useState(false);
  const status = useQuery({
    ...UFCCatalogueStatusQueryOptions(),
    refetchInterval: query => awaitingStatus || query.state.data?.running ? 1000 : query.state.data?.periodic_sync_enabled ? 30000 : false
  });
  const wasRunning = useRef(false);
  useEffect(() => {
    if (!status.data) return;
    if (lastSuccess.current !== undefined && lastSuccess.current !== status.data.last_success) {
      void queryClient.invalidateQueries({ queryKey: UFCEventsQueryOptions().queryKey });
      void queryClient.invalidateQueries({ queryKey: ["ufc", "event"] });
    }
    lastSuccess.current = status.data.last_success;
    if (awaitingStatus && !status.isFetching && !status.isError) {
      setAwaitingStatus(false);
      if (!status.data.running) {
        void queryClient.invalidateQueries({ queryKey: UFCEventsQueryOptions().queryKey });
        void queryClient.invalidateQueries({ queryKey: ["ufc", "event"] });
      }
    }
    if (wasRunning.current && !status.data.running) {
      void queryClient.invalidateQueries({ queryKey: UFCEventsQueryOptions().queryKey });
      void queryClient.invalidateQueries({ queryKey: ["ufc", "event"] });
    }
    wasRunning.current = status.data.running;
  }, [status.data, status.isFetching, status.isError, awaitingStatus, queryClient]);

  const refresh = useMutation({
    mutationFn: () => APIClient.ufc.refreshCatalogue(past ? "past" : "upcoming"),
    onSuccess: async () => {
      setAwaitingStatus(true);
      const updated = await status.refetch();
      if (!updated.isError) {
        setAwaitingStatus(false);
        if (!updated.data?.running) await refreshEvents();
      }
    }
  });
  const running = awaitingStatus || refresh.isPending || status.data?.running;
  const clear = useMutation({
    mutationFn: APIClient.ufc.clearEvents,
    onSuccess: async () => {
      setEditingID(undefined);
      setSearchID(undefined);
      setCreating(false);
      setConfirmClear(false);
      queryClient.removeQueries({ queryKey: ["ufc", "event"] });
      await queryClient.invalidateQueries({ queryKey: ["ufc"] });
    }
  });
  const reviewEvents = (events.data?.upcoming ?? []).filter(event => event.needs_review);

  return (
    <section aria-label="UFC Events" className="text-gray-900 dark:text-gray-100">
      <Section title={past ? "Past Events" : "Upcoming Events"} description={past ? "The complete saved event history. Showing 20 entries initially." : "Upcoming events and recent events awaiting components."} rightSide={
        <div className="flex flex-wrap gap-2">
        <button type="button" onClick={() => { setCreating(true); setEditingID(undefined); setSearchID(undefined); }} className="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-600 hover:text-blue-600 dark:border-gray-700 dark:text-gray-300 dark:hover:text-blue-500">New Event</button>
        <UFCActionButton busy={running || refresh.isPending} type="button" onClick={() => refresh.mutate()} disabled={running || clear.isPending} className="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-600 hover:text-blue-600 disabled:cursor-wait disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:text-blue-500">
          {running ? "Refreshing…" : "Refresh"}
        </UFCActionButton>
        {!past && <button type="button" onClick={() => { clear.reset(); setConfirmClear(true); }} disabled={showAll || running || clear.isPending} className="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-600 hover:text-red-600 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300 dark:hover:text-red-400">Clear</button>}
        </div>
      }>
        <div className="space-y-4 sm:px-2">
          {confirmClear && <div role="alert" className="space-y-3 rounded-md border border-red-300 p-3 dark:border-red-800">
            <p>Clear the active Events list? The permanent historical catalogue, library identification, manual corrections and preferences are retained. Refresh repopulates upcoming and recent events using the same IDs.</p>
            <div className="flex gap-3">
              <button type="button" disabled={running || clear.isPending} onClick={() => clear.mutate()} className="rounded-md bg-red-600 px-4 py-2 text-white hover:bg-red-700 disabled:opacity-50">{clear.isPending ? "Clearing…" : "Confirm clear Events list"}</button>
              <button type="button" disabled={clear.isPending} onClick={() => setConfirmClear(false)} className="rounded-md border border-gray-300 px-4 py-2 dark:border-gray-700">Cancel</button>
            </div>
            {clear.error && <p role="alert">Could not clear events. {clear.error.message}</p>}
          </div>}
          {running && <p role="status">Refreshing the catalogue…{status.data?.report.alias_total ? ` Aliases: ${status.data.report.alias_processed ?? 0}/${status.data.report.alias_total} checked.` : ""}</p>}
          {!past && events.isPending && <p role="status">Loading events…</p>}
          {!past && events.error && <p role="alert">Events could not be loaded. {events.error.message}</p>}
          {status.error && <p role="alert">Refresh status is unavailable. {status.error.message}</p>}
          {refresh.error && <p role="alert">Refresh could not be started. {refresh.error.message}</p>}
          {status.data?.last_error && !running && <p role="alert">Refresh failed. Saved events remain available. {status.data.last_error}</p>}
          {status.data?.interrupted && !running && <p role="status">The previous refresh was interrupted. Use Refresh to try again.</p>}
          {editingID && <UFCEventEditor key={editingID} id={editingID} onClose={() => setEditingID(undefined)} />}
          {searchID && <UFCEventSearch key={searchID} id={searchID} onClose={() => setSearchID(undefined)} />}
          {creating && <UFCNewEvent onCreated={event => { setCreating(false); setEditingID(event.id); }} onCancel={() => setCreating(false)} />}
          {showAll && <>
            <input aria-label="Find catalogue event" value={catalogueSearch} onChange={e => { setCatalogueSearch(e.target.value); }} placeholder="Find an event, alias, date or overall number" className="block w-full rounded-md border-gray-300 bg-gray-100 text-sm dark:border-gray-700 dark:bg-gray-815" />
            {catalogue.isPending && <p>Loading catalogue…</p>}{catalogue.error && <p role="alert">Could not load catalogue. {catalogue.error.message}</p>}
            {catalogue.data && <><p className="text-sm text-gray-500 dark:text-gray-400">{catalogue.data.total} saved events</p><EventGroup title="Past Events" events={catalogue.data.events} empty="No matching saved events. Use Refresh to load the complete catalogue." onEdit={id => { setCreating(false); setSearchID(undefined); setEditingID(id); }} onSearch={id => { setCreating(false); setEditingID(undefined); setSearchID(id); }} />{catalogue.data.total > catalogue.data.events.length && !loadAll && <button type="button" onClick={() => setLoadAll(true)} className="rounded-md border border-gray-300 px-4 py-2 text-sm font-medium text-gray-600 hover:text-blue-600 dark:border-gray-700 dark:text-gray-300">Load All Events</button>}</>}
          </>}
          {!showAll && events.data && <>
            <EventGroup title="Upcoming Events" events={events.data.upcoming} empty="No upcoming events saved. Use Refresh to load the catalogue." onEdit={id => { setCreating(false); setSearchID(undefined); setEditingID(id); }} onSearch={id => { setCreating(false); setEditingID(undefined); setSearchID(id); }} />
          </>}
          {status.data?.last_success && <p className="text-right text-xs text-gray-500 dark:text-gray-400">Last refreshed: <time dateTime={status.data.last_success}>{new Date(status.data.last_success).toLocaleString()}</time></p>}
          {!past && reviewEvents.length > 0 && <details>
            <summary className="cursor-pointer font-medium">Refresh entries needing review ({reviewEvents.length})</summary>
            <ul className="mt-2 list-disc space-y-1 pl-5 text-sm">{reviewEvents.map(event => <li key={event.id}>{event.name}: {(event.review ?? []).join(", ").replaceAll("_", " ")}</li>)}</ul>
          </details>}
        </div>
      </Section>
    </section>
  );
}
