/*
 * Copyright (c) 2026, UFC Manager contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { APIClient } from "@api/APIClient";
import { UFCSettingsQueryOptions } from "@api/queries";
import type { UFCComponentTemplates, UFCSettings as Settings, UFCSettingsEdit } from "@app/types/UFC";
import { Checkbox } from "@components/Checkbox";
import { componentNames, UFC_COMPONENTS, UFC_EVENT_TYPES } from "@utils/ufcComponents";
import { Section } from "./settings/_components";
import { UFCOverview } from "./UFC";
import { UFCQualityChoices } from "./UFCQuality";
import { UFCLibrarySettings } from "./UFCLibrarySettings";
import { UFCProcessingSettings } from "./UFCProcessingSettings";
import { UFCUpdates } from "./UFCUpdates";
import { UFCCatalogueRecovery } from "./UFCCatalogueRecovery";
import { compactButton, UFCCompactToggle } from "./UFCCompact";

const copyTemplates = (templates: UFCComponentTemplates): UFCComponentTemplates => ({ numbered: [...templates.numbered], fight_night: [...templates.fight_night] });
const syncValues = (settings: Settings) => ({ enabled: settings.periodic_sync_enabled ?? false, hours: String((settings.sync_interval_seconds ?? 21600) / 3600) });

function ComponentDefaults({ settings }: { settings: Settings }) {
  const queryClient = useQueryClient();
  const [base, setBase] = useState(settings);
  const [draft, setDraft] = useState(() => copyTemplates(settings.templates));
  const [syncDraft, setSyncDraft] = useState(() => syncValues(settings));
  const shippedTerms = useQuery({queryKey:["ufc","default-main-card-terms"],queryFn:APIClient.ufc.mainCardTerms});
  const [termsDraft,setTermsDraft]=useState<string|null>(settings.main_card_terms == null ? null : settings.main_card_terms.join("\n"));
  const [showTerms, setShowTerms] = useState(false);
  const termsChanged = termsDraft !== (base.main_card_terms == null ? null : base.main_card_terms.join("\n"));
  const [qualityDraft, setQualityDraft] = useState(settings.resolutions ?? ["1080p"]);
  const qualityChanged = [...qualityDraft].sort().join() !== [...(base.resolutions ?? ["1080p"])].sort().join();
  const [conflicted, setConflicted] = useState(false);
  const [latest, setLatest] = useState<Settings>();
  const [reviewError, setReviewError] = useState<string>();
  const changedTypes = UFC_EVENT_TYPES.filter(type => UFC_COMPONENTS.some(item => draft[type.key].includes(item.key) !== base.templates[type.key].includes(item.key)));
  const interval = Number(syncDraft.hours) * 3600;
  const validInterval = Number.isInteger(interval) && interval >= 3600 && interval <= 604800;
  const syncChanged = syncDraft.enabled !== (base.periodic_sync_enabled ?? false) || interval !== (base.sync_interval_seconds ?? 21600);
  const save = useMutation({
    mutationFn: () => {
      const input: UFCSettingsEdit = { revision: base.revision, templates: draft };
      if (qualityChanged) input.resolutions = qualityDraft;
      if (termsChanged) input.main_card_terms=termsDraft===null?null:termsDraft.split("\n").map(term=>term.trim()).filter(Boolean);
      if (syncChanged) { input.periodic_sync_enabled = syncDraft.enabled; input.sync_interval_seconds = interval; }
      return APIClient.ufc.saveSettings(input);
    },
    onSuccess: saved => {
      setBase(saved); setDraft(copyTemplates(saved.templates)); setConflicted(false); setLatest(undefined);
      setSyncDraft(syncValues(saved));
      setQualityDraft(saved.resolutions ?? ["1080p"]);
      setTermsDraft(saved.main_card_terms==null?null:saved.main_card_terms.join("\n"));
      queryClient.setQueryData(UFCSettingsQueryOptions().queryKey, saved);
      void queryClient.invalidateQueries({ queryKey: ["ufc", "event"] });
    },
    onError: error => { if ("status" in error && error.status === 409) setConflicted(true); }
  });
  const review = async () => {
    setReviewError(undefined);
    try { setLatest(await APIClient.ufc.settings()); }
    catch (error) { setReviewError(error instanceof Error ? error.message : "Could not load settings"); }
  };

  return <form aria-label="Catalogue settings" onSubmit={e => { e.preventDefault(); if (!conflicted && validInterval && qualityDraft.length > 0) save.mutate(); }} className="space-y-4">
    <h3 className="text-lg font-semibold">Acquisition Defaults</h3>
    <fieldset disabled={save.isPending} className="space-y-2 rounded-lg border border-gray-200 p-3 dark:border-gray-700"><legend className="px-1 font-medium">Accepted Resolutions</legend><p className="text-sm text-gray-500 dark:text-gray-400">Events inherit these choices unless overridden; no quality fallback is automatic.</p><UFCQualityChoices values={qualityDraft} disabled={save.isPending} onChange={values => { setQualityDraft(values); save.reset(); }} />{qualityDraft.length === 0 && <p role="alert">Select at least one resolution.</p>}</fieldset>
    <fieldset disabled={save.isPending} className="space-y-2 rounded-lg border border-gray-200 p-3 dark:border-gray-700"><legend className="px-1 font-medium">Main Card Exclusions</legend>
      <div className="flex min-h-9 items-end"><button type="button" aria-expanded={showTerms} onClick={() => setShowTerms(!showTerms)} className={compactButton}>{showTerms ? "Hide Excluded Terms" : "Excluded Terms"}</button></div>
      {showTerms && <>
      <p className="text-sm text-gray-500 dark:text-gray-400">Plain words or phrases, one per line. These apply to Main Card only, alongside component classification. Case and separators are normalised. Source and codec currently accept any value.</p>
      <p className="text-sm">{termsDraft===null?"Using shipped defaults":"Using custom terms"}</p>
      <label className="block text-sm">Excluded terms<textarea aria-label="Main Card exclusion terms" rows={6} value={termsDraft ?? shippedTerms.data?.flatMap(group=>group.terms).join("\n") ?? ""} disabled={save.isPending || termsDraft===null && !shippedTerms.data} onChange={e=>{setTermsDraft(e.target.value);save.reset();}} className="mt-1 block w-full rounded-md border-gray-300 bg-gray-100 dark:border-gray-700 dark:bg-gray-815" /></label>
      {shippedTerms.error && <p role="alert">Could not load shipped terms.</p>}
      <button type="button" disabled={save.isPending || termsDraft===null} onClick={()=>{setTermsDraft(null);save.reset();}} className={compactButton}>Use Default</button>
      {termsDraft==="" && <p className="text-sm text-amber-700 dark:text-amber-300">An empty custom list disables programming-term exclusions. Other component and quality checks remain.</p>}
      </>}
    </fieldset>
    <fieldset className="space-y-2 rounded-lg border border-gray-200 p-3 dark:border-gray-700"><legend className="px-1 font-medium">Wanted Components</legend>
    <p className="text-sm text-gray-500 dark:text-gray-400">Choose defaults for each event type. Individual event On/Off choices take precedence.</p>
    <div className="grid gap-4 sm:grid-cols-2">
      {UFC_EVENT_TYPES.map(type => <fieldset key={type.key} disabled={save.isPending} className="rounded-lg border border-gray-200 p-3 dark:border-gray-700">
        <legend className="px-1 font-medium">{type.label}</legend>
        {UFC_COMPONENTS.map(item => <Checkbox key={item.key} name={`${type.key}-${item.key}`} label={item.label} value={draft[type.key].includes(item.key)} disabled={save.isPending} setValue={on => {
          setDraft(current => ({ ...current, [type.key]: UFC_COMPONENTS.filter(component => component.key === item.key ? on : current[type.key].includes(component.key)).map(component => component.key) }));
          save.reset();
        }} />)}
      </fieldset>)}
    </div>
    </fieldset>
    <fieldset disabled={save.isPending} className="space-y-2 rounded-lg border border-gray-200 p-3 dark:border-gray-700">
      <legend className="px-1 font-medium">Metadata Refresh</legend>
      <p className="text-sm text-gray-500 dark:text-gray-400">Refresh upcoming events automatically. Protected edits are kept; downloads are unaffected. Use Past Events for history.</p>
      <UFCCompactToggle label="Automatic Metadata Refresh" checked={syncDraft.enabled} disabled={save.isPending} onChange={enabled => { setSyncDraft(current => ({ ...current, enabled })); save.reset(); }} />
      <label className="block text-sm font-medium">Refresh interval (hours)<input type="number" min={1} max={168} step="any" value={syncDraft.hours} onChange={e => { setSyncDraft(current => ({ ...current, hours: e.target.value })); save.reset(); }} className="mt-1 block w-full max-w-48 rounded-md border-gray-300 bg-gray-100 dark:border-gray-700 dark:bg-gray-815" /></label>
      <p className="text-sm">Saved schedule: {settings.periodic_sync_enabled ? "Enabled" : "Disabled"}{settings.periodic_sync_enabled && settings.next_due ? <> · Next refresh: <time dateTime={settings.next_due}>{new Date(settings.next_due).toLocaleString()}</time></> : ""}</p>
      {!validInterval && <p role="alert" className="text-sm text-red-600">Choose an interval between 1 and 168 hours.</p>}
      {syncChanged && validInterval && <p className="text-sm">On save: automatic metadata refresh {syncDraft.enabled ? `every ${syncDraft.hours} hours` : "disabled"}.</p>}
      <button type="submit" disabled={changedTypes.length === 0 && !syncChanged && !qualityChanged && !termsChanged || qualityDraft.length === 0 || !validInterval || conflicted || save.isPending} className={compactButton}>{save.isPending ? "Saving…" : "Save Settings"}</button>
    </fieldset>
    {changedTypes.length > 0 && <section aria-label="Default changes preview" className="space-y-2 rounded-md bg-gray-100 p-3 text-sm dark:bg-gray-750">
      <h4 className="font-medium">Changes On Save</h4>
      {changedTypes.map(type => <p key={type.key}>{type.label}: {componentNames(base.templates[type.key])} → {componentNames(draft[type.key])}. {base.event_counts[type.key]} existing events; only inherited choices change.</p>)}
      <p>Explicit event overrides will be retained.</p>
    </section>}
    {save.error && !conflicted && <p role="alert">Could not save defaults. {save.error.message}</p>}
    {save.isSuccess && <p role="status">Acquisition defaults saved.</p>}
    {conflicted && <div role="alert" className="space-y-2 rounded-md bg-amber-50 p-3 text-sm text-amber-900 dark:bg-amber-950 dark:text-amber-100">
      <p>Settings changed since this form was loaded. Your choices are retained. Review the latest revision before saving again.</p>
      <button type="button" onClick={() => void review()} className="underline">Review latest settings</button>
      {reviewError && <p>{reviewError}</p>}
      {latest && <>
        <p>Latest Main Card exclusions: {latest.main_card_terms==null?"shipped defaults":latest.main_card_terms.join(", ") || "empty custom list"}</p>
        <p>Latest accepted resolutions: {(latest.resolutions ?? ["1080p"]).join(", ")}</p>
        {UFC_EVENT_TYPES.map(type => <p key={type.key}>Latest {type.label}: {componentNames(latest.templates[type.key])}</p>)}
        <p>Latest metadata schedule: {latest.periodic_sync_enabled ? `every ${(latest.sync_interval_seconds ?? 21600) / 3600} hours` : "disabled"}</p>
        <button type="button" onClick={() => {
          const changed = new Set(changedTypes.map(type => type.key));
          setDraft(current => ({ numbered: changed.has("numbered") ? current.numbered : [...latest.templates.numbered], fight_night: changed.has("fight_night") ? current.fight_night : [...latest.templates.fight_night] }));
          if (!syncChanged) setSyncDraft(syncValues(latest));
          if (!qualityChanged) setQualityDraft(latest.resolutions ?? ["1080p"]);
          if (!termsChanged) setTermsDraft(latest.main_card_terms==null?null:latest.main_card_terms.join("\n"));
          setBase(latest); setLatest(undefined); setConflicted(false); save.reset();
        }} className="underline">Keep my choices against this revision</button>
      </>}
    </div>}
  </form>;
}

export function UFCSettings() {
  const settings = useQuery({ ...UFCSettingsQueryOptions(), refetchInterval: query => query.state.data?.periodic_sync_enabled ? 60000 : false });
  return <section aria-label="UFC Settings" className="text-gray-900 dark:text-gray-100">
    <Section title="Settings" description="Download defaults, metadata refresh and automation settings.">
      <div className="space-y-4 sm:px-2 [&_fieldset]:pt-1 [&_legend]:mb-0 [&_legend+*]:mt-0">
        {settings.isPending && <p role="status">Loading component defaults…</p>}
        {settings.error && <p role="alert">Component defaults are unavailable. {settings.error.message}</p>}
        {settings.data && <ComponentDefaults settings={settings.data} />}
        <UFCLibrarySettings />
        <UFCProcessingSettings />
        <UFCOverview />
        <UFCUpdates />
        <UFCCatalogueRecovery />
      </div>
    </Section>
  </section>;
}
