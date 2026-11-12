/*
 * Copyright (c) 2026, UFC Manager contributors.
 * SPDX-License-Identifier: GPL-2.0-or-later
 */

import type { ReactNode } from "react";
import { UFCAutomationControl, UFCProcessingAutomationControl } from "./UFCAutomation";
import { useQuery } from "@tanstack/react-query";
import { UFCStatusQueryOptions } from "@api/queries";
import { APIClient } from "@api/APIClient";
import type { UFCStatus } from "@app/types/UFC";

interface UFCStatusPanelProps {
  status?: UFCStatus;
  pending: boolean;
  error?: Error | null;
  autoCopyEnabled?: boolean;
  automationControl?: ReactNode;
  processingControl?: ReactNode;
  removalControl?: ReactNode;
}

export function UFCStatusPanel({ status, pending, error, autoCopyEnabled, automationControl, processingControl, removalControl }: UFCStatusPanelProps) {
  return (
    <fieldset aria-label="Automation Settings" className="space-y-3 rounded-lg border border-gray-200 p-3 dark:border-gray-700">
      <legend className="px-1 font-medium">Automation Settings</legend>
      {pending && <p role="status">Loading application status…</p>}
      {error && <p role="alert">Application status is unavailable. {error.message}</p>}
      {status && (
        <>
          <p role="status" className="text-xs">{status.schema_ready ? "Application Ready" : "Application Needs Attention"}</p>
          <dl className="text-xs divide-y divide-gray-200 rounded-lg border border-gray-200 dark:divide-gray-700 dark:border-gray-700">
            <div className="flex justify-between gap-4 p-3">
              <dt>Automatic Downloads</dt>
              <dd>{automationControl ?? (status.automatic_acquisition_enabled ? "Enabled" : "Disabled")}</dd>
            </div>
            <div className="flex justify-between gap-4 p-3">
              <dt>Automatic Processing</dt>
              <dd>{processingControl ?? (autoCopyEnabled === undefined ? "Unavailable" : autoCopyEnabled ? "Enabled" : "Disabled")}</dd>
            </div>
            <div className="flex justify-between gap-4 p-3"><dt>Automatic Download Removal</dt><dd>{removalControl ?? "Unavailable"}</dd></div>
          </dl>
        </>
      )}
    </fieldset>
  );
}

export function UFCOverview() {
  const { data, isPending, error } = useQuery(UFCStatusQueryOptions());
  const processing = useQuery({ queryKey: ["ufc", "processing-settings"], queryFn: APIClient.ufc.processingSettings, retry: false });
  return <UFCStatusPanel status={data} pending={isPending} error={error} autoCopyEnabled={processing.data?.auto_process_completed} automationControl={<UFCAutomationControl />} processingControl={<UFCProcessingAutomationControl field="auto_process_completed" />} removalControl={<UFCProcessingAutomationControl field="cleanup_sab_download" />} />;
}
