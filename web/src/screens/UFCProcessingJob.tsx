/* Copyright (c) 2026, UFC Manager contributors. SPDX-License-Identifier: GPL-2.0-or-later */

import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { APIClient } from "@api/APIClient";
import type { UFCProcessingJob as Job } from "@app/types/UFCProcessing";
import { compactButton } from "./UFCCompact";

export function UFCProcessingJob({ job }: { job: Job }) {
  const client = useQueryClient();
  const [confirm, setConfirm] = useState<"skip" | "keep_both" | null>(null);
  const [confirmCleanup, setConfirmCleanup] = useState(false);
  const review = useQuery({ queryKey: ["ufc", "processing-review", job.id, job.state, job.resolution], queryFn: () => APIClient.ufc.processingReview(job.id), enabled: job.state !== "completed" && !job.resolution, retry: false });
  const action = useMutation({
    mutationFn: (choice: "retry" | "skip" | "cancel" | "keep_both") => {
      switch (choice) {
        case "retry": return APIClient.ufc.retryProcessing(job.id);
        case "skip": return APIClient.ufc.skipProcessing(job.id);
        case "cancel": return APIClient.ufc.cancelProcessing(job.id);
        case "keep_both": return APIClient.ufc.keepBothProcessing(job.id, review.data?.keep_both_destination ?? "");
      }
    },
    onSuccess: () => { setConfirm(null); void client.invalidateQueries({ queryKey: ["ufc", "processing-jobs"] }); void client.invalidateQueries({ queryKey: ["ufc", "library"] }); void client.invalidateQueries({ queryKey: ["ufc", "library-coverage"] }); },
  });
  const button = compactButton;
  const cleanup = useMutation({ mutationFn: () => APIClient.ufc.retryProcessingCleanup(job.id), onSuccess: () => { setConfirmCleanup(false); void client.invalidateQueries({ queryKey: ["ufc", "processing-jobs"] }); } });
  return <section aria-label="Library Copy Job" className="space-y-2 rounded-md border border-gray-300 p-3 text-sm dark:border-gray-700">
    <p className="font-medium">Library copy: {job.resolution || job.state.replaceAll("_", " ").replace(/\b\w/g, letter => letter.toUpperCase())}</p>
    {job.message && <p className="text-gray-600 dark:text-gray-400">{job.message}</p>}
    {job.state === "completed" && job.cleanup_requested && job.cleanup_state === "needs_review" && <div className="space-y-2"><button type="button" disabled={cleanup.isPending} onClick={() => setConfirmCleanup(true)} className={button}>Retry source cleanup</button>{confirmCleanup && <div role="alert" className="space-y-2 rounded-md border border-amber-300 p-3 dark:border-amber-700"><p>Retry removal of this completed SAB source only? The app will recheck every Library copy and the exact source folder. Library files are never removed.</p><button type="button" disabled={cleanup.isPending} onClick={() => cleanup.mutate()} className={button}>Confirm cleanup retry</button><button type="button" disabled={cleanup.isPending} onClick={() => setConfirmCleanup(false)} className={button}>Cancel</button></div>}{cleanup.error && <p role="alert">Cleanup retry could not start. {cleanup.error.message}</p>}</div>}
    {job.cleanup_requested && job.cleanup_state !== "disabled" && <p className="text-gray-600 dark:text-gray-400">Source cleanup: {job.cleanup_state.replaceAll("_", " ").replace(/\b\w/g, letter => letter.toUpperCase())}{job.cleanup_message ? ` — ${job.cleanup_message}` : ""}</p>}
    {job.files.map(file => <div key={file.source} className="space-y-1"><p className="break-all">{file.source} → {file.destination} · {file.state}</p>{job.state === "running" && file.size > 0 && <progress value={file.copied_bytes} max={file.size} aria-label={`Copy progress for ${file.source}`} className="h-2 w-full" />}</div>)}
    {review.data?.reason && <p className="text-amber-700 dark:text-amber-300">{review.data.reason}</p>}
    {job.state === "needs_review" && review.data?.preferred && review.data.preferred !== "review" && <p className="text-gray-500 dark:text-gray-400">Settings preference: {review.data.preferred === "keep_both" ? "Keep Both" : "Skip"}. Confirm a choice for this file below.</p>}
    {review.error && <p role="alert">Could not load review choices. {review.error.message}</p>}
    {action.error && <p role="alert">Could not update copy job. {action.error.message}</p>}
    {review.data && <div className="flex flex-wrap gap-2">
      {review.data.retry && <button type="button" disabled={action.isPending} onClick={() => action.mutate("retry")} className={button}>Retry safely</button>}
      {review.data.keep_both_destination && <button type="button" disabled={action.isPending} onClick={() => setConfirm("keep_both")} className={button}>Keep Both</button>}
      {review.data.skip && <button type="button" disabled={action.isPending} onClick={() => setConfirm("skip")} className={button}>Skip copy</button>}
      {review.data.cancel && <button type="button" disabled={action.isPending} onClick={() => action.mutate("cancel")} className={button}>Cancel copy</button>}
    </div>}
    {confirm === "keep_both" && <div role="alert" className="space-y-2 rounded-md border border-blue-300 p-3 dark:border-blue-700"><p>Keep the existing Library file and create this separate copy:</p><p className="break-all font-medium">{review.data?.keep_both_destination}</p><button type="button" disabled={action.isPending} onClick={() => action.mutate("keep_both")} className={button}>Confirm keep both</button><button type="button" onClick={() => setConfirm(null)} className="ml-2 text-gray-600 hover:text-blue-600 dark:text-gray-300">Cancel</button></div>}
    {confirm === "skip" && <div role="alert" className="space-y-2 rounded-md border border-gray-300 p-3 dark:border-gray-700"><p>Skip this copy? The SAB source and all Library files remain untouched.</p><button type="button" disabled={action.isPending} onClick={() => action.mutate("skip")} className={button}>Confirm skip</button><button type="button" onClick={() => setConfirm(null)} className="ml-2 text-gray-600 hover:text-blue-600 dark:text-gray-300">Cancel</button></div>}
  </section>;
}
