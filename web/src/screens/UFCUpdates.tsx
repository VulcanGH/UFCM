/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { APIClient } from "@api/APIClient";
import { compactButton, UFCCompactToggle } from "./UFCCompact";

const key = ["ufc", "updates"];
const phaseLabels: Record<string, string> = {
  idle: "Not Checked", available: "Update Available", downloading: "Downloading Update",
  verified: "Ready to Update", waiting_for_idle: "Waiting for Current Work",
  quiescing: "Restarting", completed: "Update Installed", failed: "Update Needs Attention",
  deferred: "Update Postponed",
  current: "Up to Date",
  checking: "Checking for Updates…", no_release: "No Public Release Yet", check_failed: "Update Check Failed",
};

export function UFCUpdates() {
  const client = useQueryClient();
  const status = useQuery({queryKey: key, queryFn: APIClient.ufc.updateStatus, refetchInterval: 3000, retry: false});
  const check = useMutation({mutationFn: APIClient.ufc.checkUpdate, onSuccess: value => { client.setQueryData(key, value); void client.invalidateQueries({queryKey: key}); }});
  const install = useMutation({mutationFn: APIClient.ufc.installUpdate, onSuccess: value => client.setQueryData(key, value)});
  const preferences = useMutation({mutationFn: (enabled: boolean) => APIClient.ufc.saveUpdatePreferences(status.data?.revision ?? 0, enabled), onSuccess: value => client.setQueryData(key, value)});
  const value = status.data;
  const restarting = value?.phase === "quiescing";
  const error = check.error ?? install.error ?? preferences.error;
  return <fieldset className="space-y-3 rounded-md border border-gray-200 p-4 dark:border-gray-700">
    <legend className="px-1 text-sm font-medium">Updates</legend>
    <p className="text-xs text-gray-500 dark:text-gray-400">Verified updates keep your settings and library intact.</p>
    {value && <>
      <div className="flex flex-wrap items-center justify-between gap-3 text-sm">
        <div><p>Installed: {value.current}</p>{value.version && value.version.replace(/^v/, "") !== value.current.replace(/^v/, "") && <p>Available: {value.version}</p>}</div>
        <span role="status">{check.isPending ? "Checking for Updates…" : check.error ? "Update Check Failed" : phaseLabels[value.phase] ?? value.phase}</span>
      </div>
      {value.message && <p role="status" className="text-xs text-gray-500 dark:text-gray-400">{value.message}</p>}
      {!value.supported && <p className="text-xs text-gray-500 dark:text-gray-400">{value.reason}</p>}
      <div className="flex flex-wrap items-center gap-2">
        <button type="button" className={compactButton} disabled={check.isPending || value.phase === "checking" || restarting} onClick={() => check.mutate()}>{check.isPending || value.phase === "checking" ? "Checking…" : "Check for Updates"}</button>
        {value.supported && ["verified", "deferred", "waiting_for_idle", "quiescing"].includes(value.phase) && <button type="button" className={compactButton} disabled={install.isPending || !["verified", "deferred"].includes(value.phase)} onClick={() => install.mutate(value.id)}>Update and Restart</button>}
        <a className={compactButton} href={value.release_url} target="_blank" rel="noreferrer">Release Notes</a>
      </div>
      {value.supported && <UFCCompactToggle label="Automatically Install When Idle" checked={value.auto_install} disabled={preferences.isPending || restarting || value.phase === "waiting_for_idle"} onChange={enabled => preferences.mutate(enabled)} />}
    </>}
    {status.isPending && <p role="status" className="text-sm">Loading Updates…</p>}
    {status.error && <p role="status" className="text-xs">{restarting ? "Reconnecting after restart…" : "Updates unavailable. Retrying shortly…"}</p>}
    {error && <p role="alert" className="text-xs">{error.message}</p>}
  </fieldset>;
}
