import { sourceReviewFields } from "@utils/ufcReview";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { HttpClient } from "@api/APIClient";
import type { UFCEvent } from "@app/types/UFC";
import { aliasNamespaces } from "@utils/ufcAliases";
import { compactButton } from "./UFCCompact";

type Record = { id: string; name: string; date?: string; revision: number };
type Conflict = { namespace: string; value: string; records: Record[] };
type Action = Conflict & { keep?: string; recheck?: boolean };

const sourceLabels: { [reason: string]: string } = {
  article_list_date_conflict: "The Wikipedia list and article disagree on the date.",
  article_list_headliner_conflict: "The Wikipedia list and article disagree on the headliners.",
  article_list_type_conflict: "The Wikipedia list and article disagree on the event type.",
  unrecognised_date: "The source date could not be read.",
  source_date_unknown_previous_retained: "The source has no date. The last saved date was kept.",
  source_headliners_unknown_previous_retained: "The source has no headliners. The last saved headliners were kept.",
  article_enrichment_pending: "The source article still needs to be checked.",
  article_enrichment_unavailable: "The source article could not be checked. The last saved details were kept."
};

export function UFCEventReview({ event, onOpen, disabled }: { event: UFCEvent; onOpen: (id: string) => void; disabled: boolean }) {
  const client = useQueryClient();
  const id = event.id;
  const endpoint = "api/ufc/events/" + encodeURIComponent(id) + "/review";
  const queryKey = ["ufc", "event-review", id];
  const conflicts = useQuery({ queryKey, queryFn: () => HttpClient<Conflict[]>(endpoint), enabled: !!event.review?.length });
  const update = useMutation({
    mutationFn: (action: Action) => HttpClient(endpoint, {
      method: "POST",
      body: { ...action, keep: action.keep ?? "", records: action.records.map(record => ({ id: record.id, revision: record.revision })) }
    }),
    onSettled: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: ["ufc", "event-review"] }),
        client.invalidateQueries({ queryKey: ["ufc", "event"] }),
        client.invalidateQueries({ queryKey: ["ufc", "events"] }),
        client.invalidateQueries({ queryKey: ["ufc", "coverage"] })
      ]);
    }
  });
  const sourceReasons = (event.review ?? []).filter(reason => reason !== "identifier_owned_by_existing_event" && reason !== "competing_event_identifier");
  if (!event.review?.length && !update.isSuccess) return null;
  return <section aria-label="Event review" className="space-y-3 rounded-md border border-amber-300 p-4 dark:border-amber-800">
    <h4 className="font-medium">{event.review?.length ? "Needs Review" : "Review resolved"}</h4>
    {conflicts.isFetching && <p role="status" className="text-sm">Checking identifier claims…</p>}
    {conflicts.error && <p role="alert">{conflicts.error.message}</p>}
    {(event.review?.length ?? 0) > 0 && conflicts.data?.map(conflict =>
      <div key={conflict.namespace + conflict.value} className="space-y-2 text-sm">
        <p className="font-medium">Identifier clash · {aliasNamespaces.find(([key]) => key === conflict.namespace)?.[1] ?? conflict.namespace} {conflict.value}</p>
        <ul className="space-y-2">
          {conflict.records.map(record => <li key={record.id} className="flex flex-wrap items-center gap-2">
            <span>{record.name}{record.date ? " (" + record.date + ")" : ""}</span>
            {record.id !== id && <button type="button" className={compactButton} disabled={disabled || update.isPending} onClick={() => onOpen(record.id)}>Open record</button>}
            <button type="button" className={compactButton} disabled={disabled || update.isPending} onClick={() => update.mutate({ ...conflict, keep: record.id })}>Keep alias on this record</button>
          </li>)}
        </ul>
        <button type="button" className={compactButton} disabled={disabled || update.isPending} onClick={() => update.mutate({ ...conflict, recheck: true })}>Recheck from Wikipedia</button>
      </div>)}
    {sourceReasons.map(reason => <div key={reason} className="space-y-1 text-sm">
      <p><strong>{reason.startsWith("article_list_") ? "Source disagreement" : "Source needs checking"}</strong> · {sourceLabels[reason] ?? reason.replaceAll("_", " ")}</p>
      {event.source.review_evidence?.[reason]?.length ? <p>Source values: {event.source.review_evidence[reason].join(" · ")}</p> : null}
      {reason in sourceReviewFields ? <p>Correct the field below, or confirm its current value and save.</p> : null}
    </div>)}
    {update.error && <p role="alert" className="text-sm">{update.error.message}</p>}
    {update.isSuccess && <p role="status" className="text-sm">Alias ownership saved.</p>}
  </section>;
}
