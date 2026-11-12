/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */
import { useState } from "react";
import { useMutation,useQueryClient } from "@tanstack/react-query";
import { APIClient } from "@api/APIClient";
import type { UFCDownload } from "@app/types/UFCDownload";
import { UFCEventSearch } from "./UFCEventSearch";

export function UFCDownloadRecovery({job}:{job:UFCDownload}) {
 const queryClient=useQueryClient();
 const [open,setOpen]=useState(false);
 const [remoteID,setRemoteID]=useState(job.remote_id);
 const [confirmed,setConfirmed]=useState(false);
 const [searching,setSearching]=useState(false);
 const recover=useMutation({mutationFn:()=>APIClient.ufc.recoverDownload(job.id,{revision:job.revision,remote_id:remoteID.trim(),confirm:confirmed}),onSuccess:()=>{setOpen(false);setConfirmed(false);void queryClient.invalidateQueries({queryKey:["ufc","downloads"]});}});
 const button="rounded-md border border-gray-300 px-3 py-1.5 text-sm text-gray-600 hover:text-blue-600 disabled:opacity-50 dark:border-gray-700 dark:text-gray-300";
 return <div className="space-y-2">
  {job.decision?.manual && <details className="text-sm"><summary className="cursor-pointer">Manual assignment</summary><p>{job.decision.reason}</p><p className="text-gray-500 dark:text-gray-400">{job.decision.actor} · {new Date(job.decision.at).toLocaleString()} · Original identity: {job.decision.automatic.status} · Components: {job.decision.automatic.components.values.join(", ") || "unknown"}</p></details>}
  {["uncertain","missing","needs_review","submitting"].includes(job.state) && <button type="button" className={button} onClick={()=>{setOpen(!open);recover.reset();}}>Recover existing SAB job</button>}
  {open && <form className="space-y-2 rounded-md border border-gray-300 p-3 text-sm dark:border-gray-700" onSubmit={e=>{e.preventDefault();if(confirmed && remoteID.trim())recover.mutate();}}>
   <p>Bind an existing SAB job on the original server. Its title must match this release. This only checks status and saves the binding; it never sends another download, cancels a job or changes files.</p>
   <label className="block">SAB job ID<input aria-label="SAB job ID" value={remoteID} maxLength={256} disabled={recover.isPending} onChange={e=>{setRemoteID(e.target.value);setConfirmed(false);recover.reset();}} className="mt-1 block w-full rounded-md border-gray-300 bg-gray-100 dark:border-gray-700 dark:bg-gray-815" /></label>
   <label className="flex items-center gap-2"><input type="checkbox" checked={confirmed} disabled={recover.isPending} onChange={e=>setConfirmed(e.target.checked)} />I checked this is the same release in SABnzbd.</label>
   <button type="submit" className={button} disabled={recover.isPending || !confirmed || !remoteID.trim()}>{recover.isPending?"Verifying…":"Verify and bind"}</button>
   {recover.error && <p role="alert">{recover.error.message} Your input is retained; check the current status before trying again.</p>}
  </form>}
  {job.state==="failed" && <><p className="text-sm text-gray-500 dark:text-gray-400">SAB reports a definite failure. A retry requires a fresh search and confirmation; this record is retained.</p><button type="button" className={button} onClick={()=>setSearching(!searching)}>{searching?"Close retry search":"Manual Search to retry"}</button>{searching && <UFCEventSearch id={job.event_id} onClose={()=>setSearching(false)} />}</>}
 </div>;
}
