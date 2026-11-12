/*
 * Copyright (c) 2026, UFC Manager contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { useEffect, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { APIClient } from "@api/APIClient";
import { UFCEventQueryOptions, UFCEventsQueryOptions } from "@api/queries";
import type { UFCEditableField, UFCEvent, UFCEventEdit, UFCEventValues, UFCComponentKey, UFCComponentChoice, UFCAliasEdit } from "@app/types/UFC";
import { UFCAliases } from "./UFCAliases";
import { UFCQualityChoices } from "./UFCQuality";
import { compactButton } from "./UFCCompact";
import type { UFCQualityEdit } from "@app/types/UFC";
import { describeAliases, orderedAliases } from "@utils/ufcAliases";
import { componentNames, UFC_COMPONENTS } from "@utils/ufcComponents";
import { useAppForm } from "@hooks/form";
import { TextField } from "@components/inputs";
import { eventStatusLabel } from "@utils/ufcEventStatus";

const fields = ["name", "type", "date", "headliners", "status"] as const;
const labels: Record<UFCEditableField, string> = { name: "Name", type: "Event type", date: "Date", headliners: "Headliners", status: "Status" };
const display = (values: UFCEventValues, key: UFCEditableField) => {
  const value = values[key];
  if (key === "status" && typeof value === "string") return eventStatusLabel(value);
  if (key === "type") return value === "numbered" ? "Numbered UFC" : value === "fight_night" ? "Fight Night" : "Unknown";
  return Array.isArray(value) ? value.join(" vs. ") || "Unknown" : value || "Unknown";
};
const defaults = (event: UFCEvent) => ({
  name: event.values.name, type: event.values.type, date: event.values.date ?? "",
  first: event.values.headliners?.[0] ?? "", second: event.values.headliners?.[1] ?? "", status: event.values.status
});

function EditForm({ event, onClose }: { event: UFCEvent; onClose: () => void }) {
  const queryClient = useQueryClient();
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => { heading.current?.focus(); }, []);
  const [base, setBase] = useState(event);
  const [operations, setOperations] = useState<Partial<Record<UFCEditableField, "set" | "reset">>>({});
  const [componentOperations, setComponentOperations] = useState<Partial<Record<UFCComponentKey, UFCComponentChoice>>>({});
  const [aliasOperation, setAliasOperation] = useState<UFCAliasEdit>();
  const [qualityOperation, setQualityOperation] = useState<UFCQualityEdit>();
  const [conflicted, setConflicted] = useState(false);
  const [latest, setLatest] = useState<UFCEvent>();
  const [reviewError, setReviewError] = useState<string>();
  const [confirmDelete, setConfirmDelete] = useState(false);
  const remove = useMutation({
    mutationFn: () => APIClient.ufc.deleteEvent(event.id, base.revision),
    onSuccess: () => {
      onClose();
      void queryClient.invalidateQueries({ queryKey: UFCEventsQueryOptions().queryKey });
      queryClient.removeQueries({ queryKey: UFCEventQueryOptions(event.id).queryKey });
    },
    onError: error => {
      if ("status" in error && error.status === 409) setConflicted(true);
    }
  });
  const form = useAppForm({
    defaultValues: defaults(event),
    onSubmit: async ({ value }) => {
      if (conflicted) return;
      const edit: UFCEventEdit = { revision: base.revision };
      if (qualityOperation) edit.quality = qualityOperation;
      if (Object.keys(componentOperations).length) edit.components = componentOperations;
      if (aliasOperation) edit.aliases = aliasOperation;
      for (const key of fields) {
        const operation = operations[key];
        if (!operation) continue;
        edit[key] = operation === "reset" ? { operation } : key === "headliners"
          ? { operation, names: [value.first.trim(), value.second.trim()].filter(Boolean) }
          : { operation, value: key === "date" ? value.date || null : value[key].trim() };
      }
      await save.mutateAsync(edit).catch(() => undefined);
    }
  });
  const save = useMutation({
    mutationFn: (edit: UFCEventEdit) => APIClient.ufc.editEvent(event.id, edit),
    onSuccess: saved => {
      setBase(saved);
      setOperations({});
      setComponentOperations({});
      setAliasOperation(undefined);
      setQualityOperation(undefined);
      setLatest(undefined);
      form.reset(defaults(saved));
      queryClient.setQueryData(UFCEventQueryOptions(event.id).queryKey, saved);
      void queryClient.invalidateQueries({ queryKey: UFCEventsQueryOptions().queryKey });
    },
    onError: error => {
      if ("status" in error && error.status === 409) setConflicted(true);
    }
  });
  const stage = (key: UFCEditableField, operation: "set" | "reset") => {
    setOperations(current => ({ ...current, [key]: operation }));
    save.reset();
  };
  const reset = (key: UFCEditableField) => {
    stage(key, "reset");
    if (key === "headliners") {
      form.setFieldValue("first", event.source.headliners?.[0] ?? "");
      form.setFieldValue("second", event.source.headliners?.[1] ?? "");
    } else if (key === "status") form.setFieldValue(key, event.source.status);
    else if (key === "type") form.setFieldValue(key, event.source.type);
    else form.setFieldValue(key, event.source[key] ?? "");
  };
  const reviewLatest = async () => {
    setReviewError(undefined);
    try {
      const current = await APIClient.ufc.event(event.id);
      setLatest(current);
      queryClient.setQueryData(UFCEventQueryOptions(event.id).queryKey, current);
    }
    catch (error) { setReviewError(error instanceof Error ? error.message : "Could not load latest revision"); }
  };
  const changed = Object.keys(operations).length > 0 || Object.keys(componentOperations).length > 0 || !!aliasOperation || !!qualityOperation;
  const qualityCustom = qualityOperation ? qualityOperation.mode === "custom" : !!event.quality?.override;
  const qualityDefaults = event.quality?.defaults ?? ["1080p"];
  const qualityValues = qualityOperation ? qualityOperation.mode === "custom" ? qualityOperation.resolutions ?? [] : qualityDefaults : event.quality?.effective ?? qualityDefaults;
  const aliases = aliasOperation ? aliasOperation.operation === "reset" ? orderedAliases(event.source.aliases) : aliasOperation.values ?? [] : orderedAliases(event.aliases);
  const componentChoice = (key: UFCComponentKey): UFCComponentChoice => componentOperations[key] ?? (event.components?.overrides[key] === undefined ? "inherit" : event.components.overrides[key] ? "on" : "off");
  const wanted = UFC_COMPONENTS.filter(item => componentChoice(item.key) === "inherit" ? event.components?.defaults.includes(item.key) : componentChoice(item.key) === "on").map(item => item.key);

  return <form.AppForm>
    <form onSubmit={e => { e.preventDefault(); e.stopPropagation(); void form.handleSubmit(); }} className="space-y-4" aria-label="Edit event">
      <div className="flex items-start justify-between gap-3"><h3 ref={heading} tabIndex={-1} className="text-lg font-semibold">Edit Event</h3><button type="button" onClick={onClose} disabled={save.isPending || remove.isPending} className={compactButton}>Close Editor</button></div>
      <p className="text-sm text-gray-600 dark:text-gray-400">Changes are protected from refresh by default. A blank date or both blank headliners saves a protected unknown value. {event.source.page_id > 0 ? "Use default restores the Wikipedia value and removes protection when you save." : "This is a local event without a Wikipedia link; it is not automatically merged with imported events."}</p>
      <fieldset disabled={save.isPending} className="space-y-4">
        {fields.map(key => <div key={key} className="space-y-2 rounded-md border border-gray-200 p-3 dark:border-gray-700">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="font-medium">{labels[key]} <span className="text-xs font-normal">{operations[key] === "reset" ? "Default (on save)" : operations[key] === "set" ? "Protected (on save)" : event.corrections[key]?.protected ? "Protected" : "Default"}</span></span>
            {event.source.page_id > 0 && <button type="button" onClick={() => reset(key)} aria-label={`Use default for ${labels[key]}`} className={compactButton}>Use Default</button>}
          </div>
          <p className="text-sm">Saved value: {display(event.values, key)}</p>
          {event.source.page_id > 0 && display(event.source, key) !== display(event.values, key) && <p className="text-sm text-amber-800 dark:text-amber-300">Wikipedia suggestion: {display(event.source, key)}</p>}
          <div onChange={() => stage(key, "set")}>
            {key === "headliners" ? <div className="grid grid-cols-12 gap-3"><TextField name="first" label="First Headliner" columns={6} /><TextField name="second" label="Second Headliner" columns={6} /></div>
              : key === "name" ? <TextField name="name" label="Event Name" />
                : key === "type" ? <form.Field name="type">{field => <><label htmlFor="event-type" className="text-sm">Event type</label><select id="event-type" value={field.state.value} onChange={e => field.handleChange(e.target.value as UFCEventValues["type"])} className="mt-1 block w-full rounded-md border-gray-300 bg-gray-100 dark:border-gray-700 dark:bg-gray-815"><option value="numbered">Numbered UFC</option><option value="fight_night">Fight Night</option></select><p className="mt-1 text-sm text-gray-500 dark:text-gray-400">Changing type changes inherited component defaults after saving. Explicit On/Off choices are retained.</p></>}</form.Field>
                : key === "date" ? <form.Field name="date">{field => <><label htmlFor="event-date" className="text-sm">Event date</label><input id="event-date" type="date" value={field.state.value} onChange={e => field.handleChange(e.target.value)} className="mt-1 block w-full rounded-md border-gray-300 bg-gray-100 dark:border-gray-700 dark:bg-gray-815" /></>}</form.Field>
                  : <form.Field name="status">{field => <><label htmlFor="event-status" className="text-sm">Event status</label><select id="event-status" value={field.state.value} onChange={e => field.handleChange(e.target.value as UFCEventValues["status"])} className="mt-1 block w-full rounded-md border-gray-300 bg-gray-100 dark:border-gray-700 dark:bg-gray-815">{["unknown", "scheduled", "postponed", "cancelled", "completed"].map(value => <option key={value} value={value}>{eventStatusLabel(value)}</option>)}</select></>}</form.Field>}
          </div>
        </div>)}
      </fieldset>
      <section aria-label="Event aliases" className="space-y-3 rounded-md border border-gray-200 p-3 dark:border-gray-700">
        <div className="flex flex-wrap items-center justify-between gap-2"><h4 className="font-medium">Aliases <span className="text-xs font-normal">{aliasOperation?.operation === "reset" ? "Default (on save)" : aliasOperation || event.corrections.aliases?.protected ? "Protected" : "Default"}</span></h4>
          {event.source.page_id > 0 && <button type="button" disabled={save.isPending} onClick={() => { setAliasOperation({ operation: "reset" }); save.reset(); }} className={compactButton}>Use Default</button>}
        </div>
        <p className="text-sm">Saved aliases: {describeAliases(event.aliases)}</p>
        {event.source.page_id > 0 && describeAliases(event.aliases) !== describeAliases(event.source.aliases) && <p className="text-sm text-amber-800 dark:text-amber-300">Wikipedia suggestion: {describeAliases(event.source.aliases)}</p>}
        <details><summary className="cursor-pointer text-sm font-medium">Edit Aliases</summary><div className="mt-3"><UFCAliases values={aliases} disabled={save.isPending} onChange={values => { setAliasOperation({ operation: "set", values }); save.reset(); }} /></div></details>
      </section>
      {event.components && <fieldset disabled={save.isPending} className="space-y-3 rounded-md border border-gray-200 p-3 dark:border-gray-700">
        <legend className="px-1 font-medium">Wanted Components</legend>
        <p className="text-sm text-gray-500 dark:text-gray-400">Inherit follows the current {event.values.type === "numbered" ? "numbered event" : "Fight Night"} defaults. On and Off are local overrides retained through refresh.</p>
        <div className="grid gap-3 sm:grid-cols-2">
          {UFC_COMPONENTS.map(item => <div key={item.key}>
            <label htmlFor={`component-${item.key}`} className="text-sm font-medium">{item.label}</label>
            <select id={`component-${item.key}`} value={componentChoice(item.key)} onChange={e => {
              const choice = e.target.value as UFCComponentChoice;
              setComponentOperations(current => ({ ...current, [item.key]: choice })); save.reset();
            }} className="mt-1 block w-full rounded-md border-gray-300 bg-gray-100 dark:border-gray-700 dark:bg-gray-815">
              <option value="inherit">Inherit ({event.components?.defaults.includes(item.key) ? "On" : "Off"})</option>
              <option value="on">On</option><option value="off">Off</option>
            </select>
          </div>)}
        </div>
        <p className="text-sm">Wanted on save: {componentNames(wanted)}</p>
      </fieldset>}
      <fieldset disabled={save.isPending} className="space-y-3 rounded-md border border-gray-200 p-3 dark:border-gray-700"><legend className="px-1 font-medium">Accepted Resolutions</legend>
        <UFCQualityChoices values={qualityValues} disabled={save.isPending} onChange={values => { setQualityOperation({ mode: "custom", resolutions: values }); save.reset(); }} />
        <div className="flex flex-wrap items-center gap-3 text-sm"><p className="text-gray-500 dark:text-gray-400">{qualityCustom ? "Event override" : "Following Settings defaults"}{qualityOperation ? " (on save)" : ""}</p><button type="button" disabled={!qualityCustom || save.isPending} onClick={() => { setQualityOperation({ mode: "inherit" }); save.reset(); }} className="text-gray-600 hover:text-blue-600 disabled:opacity-50 dark:text-gray-300 dark:hover:text-blue-500">Use Settings defaults ({qualityDefaults.join(", ")})</button></div>
        {qualityCustom && qualityValues.length === 0 && <p role="alert">Select at least one resolution.</p>}
      </fieldset>
      {save.error && !conflicted && <p role="alert">Could not save event. {save.error.message}</p>}
      {save.isSuccess && <p role="status">Event saved.</p>}
      {conflicted && <div role="alert" className="space-y-3 rounded-md bg-amber-50 p-3 text-amber-900 dark:bg-amber-950 dark:text-amber-100">
        <p>This event has a newer revision. Your unsaved inputs are retained. Review the latest values before saving again.</p>
        <button type="button" onClick={() => void reviewLatest()} className="underline">Review latest revision</button>
        {reviewError && <p>{reviewError}</p>}
        {latest && <>
          <p>Latest saved revision: {latest.revision}</p>
          <ul>{fields.map(key => <li key={key}>{labels[key]}: {display(latest.values, key)}{latest.corrections[key]?.protected ? " (protected)" : " (Wikipedia)"}{latest.source.page_id > 0 && display(latest.source, key) !== display(latest.values, key) ? `; Wikipedia: ${display(latest.source, key)}` : ""}</li>)}</ul>
          <p>Latest aliases: {describeAliases(latest.aliases)}</p>
          <p>Latest resolutions: {latest.quality?.effective.join(", ") ?? "1080p"} ({latest.quality?.override ? "event override" : "inherited"})</p>
          {latest.components && <><p>Latest wanted components: {componentNames(latest.components.wanted)}</p><p>Latest overrides: {Object.entries(latest.components.overrides).map(([key, on]) => `${componentNames([key])}: ${on ? "On" : "Off"}`).join(", ") || "All inherited"}</p></>}
          <button type="button" onClick={() => {
            for (const key of fields) {
              if (operations[key] === "set") continue;
              const values = operations[key] === "reset" ? latest.source : latest.values;
              if (key === "headliners") {
                form.setFieldValue("first", values.headliners?.[0] ?? "");
                form.setFieldValue("second", values.headliners?.[1] ?? "");
              } else if (key === "status") form.setFieldValue(key, values.status);
              else if (key === "type") form.setFieldValue(key, values.type);
              else form.setFieldValue(key, values[key] ?? "");
            }
            setBase(latest); setLatest(undefined); setConflicted(false); save.reset();
          }} className="underline">Keep My Draft Against This Revision</button>
        </>}
      </div>}
      <div className="flex flex-wrap gap-3">
        <button type="button" disabled={save.isPending || remove.isPending || conflicted} onClick={() => setConfirmDelete(true)} className="rounded-md border border-red-300 px-2.5 py-1 text-xs font-medium text-red-600 hover:bg-red-50 dark:border-red-800 dark:hover:bg-red-950">Delete Event</button>
        <button type="submit" disabled={!changed || (qualityCustom && qualityValues.length === 0) || save.isPending || remove.isPending || conflicted} className="rounded-md bg-blue-600 px-2.5 py-1 text-xs font-medium leading-5 text-white hover:bg-blue-700 disabled:opacity-50">{save.isPending ? "Saving…" : "Save Corrections"}</button>
      </div>
      {confirmDelete && <div role="alert" className="space-y-3 rounded-md border border-red-300 p-3 dark:border-red-800">
        <p>Delete {base.values.name}? It will be removed from the catalogue and Wikipedia refresh will not restore it. Media files are unaffected.</p>
        <div className="flex gap-3">
          <button type="button" disabled={remove.isPending || save.isPending || conflicted} onClick={() => remove.mutate()} className="rounded-md bg-red-600 px-4 py-2 text-white hover:bg-red-700 disabled:opacity-50">{remove.isPending ? "Deleting…" : "Confirm delete"}</button>
          <button type="button" disabled={remove.isPending} onClick={() => setConfirmDelete(false)} className="rounded-md border border-gray-300 px-4 py-2 dark:border-gray-700">Cancel</button>
        </div>
        {remove.error && <p role="alert">Could not delete event. {remove.error.message}</p>}
      </div>}
    </form>
  </form.AppForm>;
}

export function UFCEventEditor({ id, onClose }: { id: string; onClose: () => void }) {
  const event = useQuery({ ...UFCEventQueryOptions(id), refetchOnWindowFocus: false });
  return <section aria-label="Event editor" className="space-y-3 rounded-lg border border-gray-300 p-4 dark:border-gray-600">
    {event.isPending && <p role="status">Loading event…</p>}
    {event.error && <><p role="alert">Event could not be loaded. {event.error.message}</p><button type="button" onClick={onClose} className={compactButton}>Close Editor</button></>}
    {event.data && <EditForm key={id} event={event.data} onClose={onClose} />}
  </section>;
}
