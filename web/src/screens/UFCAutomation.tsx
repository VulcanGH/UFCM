/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */

import { useIsMutating, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { APIClient } from "@api/APIClient";
import { compactButton, UFCCompactToggle } from "./UFCCompact";

export function UFCAutomationControl() {
  const client = useQueryClient();
  const settings = useQuery({ queryKey: ["ufc", "automation-settings"], queryFn: APIClient.ufc.automationSettings, retry: false });
  const save = useMutation({
    mutationFn: (enabled: boolean) => APIClient.ufc.saveAutomationSettings({ revision: settings.data?.revision ?? 0, enabled }),
    onSuccess: saved => { client.setQueryData(["ufc", "automation-settings"], saved); void client.invalidateQueries({ queryKey: ["ufc", "status"] }); },
  });
  return <div className="space-y-2">
    {settings.data && <UFCCompactToggle label={settings.data.enabled ? "Enabled" : "Disabled"} checked={settings.data.enabled} disabled={save.isPending} onChange={enabled => { save.reset(); save.mutate(enabled); }} />}
    {settings.isPending && <span>Loading…</span>}
    {settings.error && <span role="alert">Automation unavailable</span>}
    {save.error && <div role="alert" className="space-y-1 text-xs"><p>Could not update automation. {save.error.message}</p><button type="button" className={compactButton} onClick={() => { void settings.refetch(); save.reset(); }}>Reload Current Setting</button></div>}
  </div>;
}

export function UFCProcessingAutomationControl({field}:{field:"auto_process_completed"|"cleanup_sab_download"}) {
 const client=useQueryClient();
 const settings=useQuery({queryKey:["ufc","processing-settings"],queryFn:APIClient.ufc.processingSettings,retry:false});
 const pending=useIsMutating({mutationKey:["ufc","processing-settings"]})>0;
 const save=useMutation({mutationKey:["ufc","processing-settings"],mutationFn:(enabled:boolean)=>{if(!settings.data)throw new Error("Processing settings unavailable");return APIClient.ufc.saveProcessingSettings({...settings.data,[field]:enabled});},onSuccess:saved=>{client.setQueryData(["ufc","processing-settings"],saved);}});
 return <div className="space-y-2">{settings.data&&<UFCCompactToggle label={settings.data[field]?"Enabled":"Disabled"} checked={settings.data[field]} disabled={pending} onChange={value=>{save.reset();save.mutate(value);}}/>}{settings.isPending&&<span>Loading?</span>}{settings.error&&<span role="alert">Unavailable</span>}{save.error&&<div role="alert" className="space-y-1 text-xs"><p>Could not save this setting. {save.error.message}</p><button type="button" className={compactButton} onClick={()=>{void settings.refetch();save.reset();}}>Reload Current Setting</button></div>}</div>;
}
