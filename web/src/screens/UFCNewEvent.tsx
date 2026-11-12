/*
 * Copyright (c) 2026, UFC Manager contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { APIClient } from "@api/APIClient";
import type { UFCEvent, UFCEventAlias, UFCEventCreate, UFCEventValues } from "@app/types/UFC";
import { useAppForm } from "@hooks/form";
import { TextField } from "@components/inputs";
import { eventStatusLabel } from "@utils/ufcEventStatus";
import { UFCAliases } from "./UFCAliases";
import { compactButton } from "./UFCCompact";

const inputStyle = "mt-1 block w-full rounded-md border-gray-300 bg-gray-100 dark:border-gray-700 dark:bg-gray-815";

export function UFCNewEvent({ onCreated, onCancel }: { onCreated: (event: UFCEvent) => void; onCancel: () => void }) {
  const queryClient = useQueryClient();
  const [aliases, setAliases] = useState<UFCEventAlias[]>([]);
  const create = useMutation({
    mutationFn: (input: UFCEventCreate) => APIClient.ufc.createEvent(input),
    onSuccess: event => { void queryClient.invalidateQueries({ queryKey: ["ufc"] }); onCreated(event); }
  });
  const form = useAppForm({
    defaultValues: { name: "", type: "numbered" as UFCEventValues["type"], date: "", first: "", second: "", status: "scheduled" as UFCEventValues["status"] },
    onSubmit: async ({ value }) => {
      await create.mutateAsync({ values: { name: value.name.trim(), type: value.type, date: value.date || null, headliners: [value.first.trim(), value.second.trim()].filter(Boolean), status: value.status }, aliases }).catch(() => undefined);
    }
  });
  return <section aria-label="New event" className="space-y-4 rounded-lg border border-gray-300 p-4 dark:border-gray-600">
    <div className="flex items-start justify-between gap-3"><h3 className="text-lg font-semibold">New Local Event</h3><button type="button" onClick={onCancel} disabled={create.isPending} className={compactButton}>Close Editor</button></div>
    <p className="text-sm text-gray-500 dark:text-gray-400">Add an event missing from the catalogue. Its values are protected, with a separate local ID and no Wikipedia link. It will not automatically merge with an imported event.</p>
    <form.AppForm><form onSubmit={e => { e.preventDefault(); void form.handleSubmit(); }} className="space-y-4">
      <fieldset disabled={create.isPending} className="space-y-4">
        <TextField name="name" label="Event Name" required />
        <div className="grid gap-3 sm:grid-cols-2">
          <form.Field name="type">{field => <label className="text-sm">Event type<select value={field.state.value} onChange={e => field.handleChange(e.target.value as UFCEventValues["type"])} className={inputStyle}><option value="numbered">Numbered UFC</option><option value="fight_night">Fight Night</option></select></label>}</form.Field>
          <form.Field name="date">{field => <label className="text-sm">Event date<input type="date" value={field.state.value} onChange={e => field.handleChange(e.target.value)} className={inputStyle} /></label>}</form.Field>
        </div>
        <div className="grid grid-cols-12 gap-3"><TextField name="first" label="First Headliner" columns={6} /><TextField name="second" label="Second Headliner" columns={6} /></div>
        <form.Field name="status">{field => <label className="block text-sm">Event status<select value={field.state.value} onChange={e => field.handleChange(e.target.value as UFCEventValues["status"])} className={inputStyle}>{["unknown", "scheduled", "postponed", "cancelled", "completed"].map(status => <option key={status} value={status}>{eventStatusLabel(status)}</option>)}</select></label>}</form.Field>
        <details><summary className="cursor-pointer font-medium">Aliases (optional)</summary><div className="mt-3"><UFCAliases values={aliases} onChange={setAliases} disabled={create.isPending} /></div></details>
      </fieldset>
      {create.error && <p role="alert">Event could not be created. {create.error.message}</p>}
      <div className="flex flex-wrap gap-3"><button type="submit" disabled={create.isPending} className="rounded-md bg-blue-600 px-2.5 py-1 text-xs font-medium leading-5 text-white hover:bg-blue-700 disabled:opacity-50">{create.isPending ? "Creating…" : "Create Event"}</button></div>
    </form></form.AppForm>
  </section>;
}
