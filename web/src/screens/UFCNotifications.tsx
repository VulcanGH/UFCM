import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { HttpClient } from "@api/APIClient";
import { compactButton } from "./UFCCompact";
import { Section } from "./settings/_components";

const events = {
  UFC_COPY_COMPLETED: "Library copy completed",
  UFC_DOWNLOAD_COMPLETED: "Download completed",
  UFC_ATTENTION: "Needs attention",
  APP_UPDATE_AVAILABLE: "Application update available"
};
type Event = keyof typeof events;
type Format = { enabled: boolean; title: string; message: string; color: number };
type Settings = { revision: number; enabled: boolean; configured: boolean; events: Record<Event, Format> };
type Delivery = { id: string; event: Event; outcome: string; reason: string; time: string };

const endpoint = "api/ufc/notifications";
const historyKey = ["ufc", "notification-history"];
const input = "w-full rounded border-gray-300 bg-gray-100 dark:border-gray-700 dark:bg-gray-815";
const eventKeys = Object.keys(events) as Event[];
const sample = { name: "UFC Fight Night: Sample Event", message: "This is a UFC Manager test notification." };

function Editor({ initial, onSave }: { initial: Settings; onSave: (settings: Settings) => void }) {
  const client = useQueryClient();
  const [draft, setDraft] = useState(initial);
  const [webhook, setWebhook] = useState<string>();
  const [selected, setSelected] = useState<Event>("UFC_COPY_COMPLETED");
  const format = draft.events[selected];
  const changeFormat = (patch: Partial<Format>) => setDraft(current => ({
    ...current, events: { ...current.events, [selected]: { ...current.events[selected], ...patch } }
  }));
  const save = useMutation({
    mutationFn: () => HttpClient<Settings>(endpoint, { method: "PUT", body: { ...draft, webhook } }),
    onSuccess: saved => {
      setDraft(saved);
      setWebhook(undefined);
      onSave(saved);
    }
  });
  const test = useMutation({
    mutationFn: () => HttpClient(endpoint + "/test", { method: "POST", body: { event: selected } }),
    onSettled: () => { void client.invalidateQueries({ queryKey: historyKey }); }
  });
  const changed = webhook !== undefined || JSON.stringify(initial) !== JSON.stringify(draft);
  const color = "#" + format.color.toString(16).padStart(6, "0");
  const preview = (value: string) => value.replace(/\{(name|message|event)\}/g, (_, key: keyof typeof sample | "event") => key === "event" ? selected : sample[key]);
  return <form className="space-y-5" onSubmit={e => { e.preventDefault(); save.mutate(); }}>
    <fieldset disabled={save.isPending} className="space-y-4 rounded-md border border-gray-200 p-4 dark:border-gray-700">
      <legend className="px-1 font-medium">Discord</legend>
      <label className="flex items-center gap-2"><input type="checkbox" checked={draft.enabled} onChange={e => setDraft({ ...draft, enabled: e.target.checked })} /> Enable Discord delivery</label>
      <label className="block space-y-1"><span>Webhook URL</span>
        <input className={input} type="password" autoComplete="off" value={webhook ?? ""} placeholder={initial.configured ? "Saved webhook" : "https://discord.com/api/webhooks/…"} onChange={e => setWebhook(e.target.value)} />
      </label>
      {initial.configured && <button type="button" className={compactButton} onClick={() => { setWebhook(""); setDraft({ ...draft, enabled: false }); }}>Clear saved webhook</button>}
      {webhook === "" && <p className="text-sm">Save to clear the webhook and disable delivery.</p>}
    </fieldset>
    <fieldset disabled={save.isPending} className="space-y-4 rounded-md border border-gray-200 p-4 dark:border-gray-700">
      <legend className="px-1 font-medium">Events and embed</legend>
      <div className="grid gap-2 sm:grid-cols-2">{eventKeys.map(key =>
        <label key={key} className="flex items-center gap-2 text-sm">
          <input type="checkbox" checked={draft.events[key].enabled} onChange={e => {
            const enabled = e.target.checked;
            setDraft(current => ({ ...current, events: { ...current.events, [key]: { ...current.events[key], enabled } } }));
          }} /> {events[key]}
        </label>)}</div>
      <label className="block space-y-1"><span>Embed to edit</span>
        <select className={input} value={selected} onChange={e => { setSelected(e.target.value as Event); test.reset(); }}>{eventKeys.map(key => <option key={key} value={key}>{events[key]}</option>)}</select>
      </label>
      <label className="block space-y-1"><span>Title</span><input className={input} required maxLength={256} value={format.title} onChange={e => changeFormat({ title: e.target.value })} /></label>
      <label className="block space-y-1"><span>Message</span><textarea className={input} rows={3} required maxLength={1500} value={format.message} onChange={e => changeFormat({ message: e.target.value })} /></label>
      <p className="text-sm text-gray-500 dark:text-gray-400">Fields you can insert: {"{name}"}, {"{message}"}, {"{event}"}.</p>
      <label className="flex items-center gap-2"><input type="color" value={color} onChange={e => changeFormat({ color: parseInt(e.target.value.slice(1), 16) })} /> Embed colour</label>
      <div aria-label="Embed preview" className="break-words rounded border-l-4 bg-gray-800 p-4 text-sm text-gray-100" style={{ borderColor: color }}>
        <p className="font-semibold">{preview(format.title)}</p>
        <p className="mt-1 whitespace-pre-wrap">{preview(format.message)}</p>
      </div>
    </fieldset>
    <div className="flex flex-wrap gap-2">
      <button className={compactButton} disabled={!changed || save.isPending}>Save notifications</button>
      <button className={compactButton} type="button" disabled={!initial.configured || changed || test.isPending} onClick={() => test.mutate()}>Send test</button>
    </div>
    {save.error && <p role="alert">{save.error.message}</p>}
    {test.error && <p role="alert">{test.error.message}</p>}
    {save.isSuccess && !changed && <p role="status">Settings saved.</p>}
    {test.isSuccess && <p role="status">Test sent.</p>}
  </form>;
}

export function UFCNotifications() {
  const client = useQueryClient();
  const settingsKey = ["ufc", "notification-settings"];
  const settings = useQuery({ queryKey: settingsKey, queryFn: () => HttpClient<Settings>(endpoint) });
  const history = useQuery({ queryKey: historyKey, queryFn: () => HttpClient<Delivery[]>(endpoint + "/history"), refetchInterval: 15000 });
  return <Section title="Notifications" description="Choose what UFC Manager sends to Discord.">
    {settings.isPending && <p role="status">Loading settings…</p>}
    {settings.error && <p role="alert">{settings.error.message}</p>}
    {settings.data && <Editor initial={settings.data} onSave={saved => client.setQueryData(settingsKey, saved)} />}
    <section className="mt-6 space-y-2" aria-label="Recent deliveries">
      <h3 className="font-medium">Recent deliveries</h3>
      {history.error && <p role="alert">{history.error.message}</p>}
      {history.data?.length === 0 && <p className="text-sm">No deliveries recorded yet.</p>}
      <ul className="divide-y divide-gray-200 text-sm dark:divide-gray-700">{history.data?.map(item =>
        <li key={item.id} className="py-2">
          <time dateTime={item.time}>{new Date(item.time).toLocaleString()}</time>
          {" · "}{events[item.event] ?? item.event}{" · "}{item.outcome}{" · "}{item.reason}
        </li>)}</ul>
    </section>
  </Section>;
}
