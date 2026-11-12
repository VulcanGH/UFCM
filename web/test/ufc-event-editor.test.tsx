import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";

import { APIClient } from "@api/APIClient";
import { UFCEventQueryOptions } from "@api/queries";
import type { UFCEvent } from "@app/types/UFC";
import { UFCEventEditor } from "@screens/UFCEventEditor";
import { UFCEvents } from "@screens/UFCEvents";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

function fixture(): UFCEvent {
  return {
    id: "stable-local", revision: 2,
    values: { name: "My event", type: "numbered", date: null, headliners: ["Manual One", "Manual Two"], status: "postponed" },
    source: { name: "Wikipedia event", type: "numbered", date: "2027-01-30", headliners: ["Source One", "Source Two"], status: "scheduled", page_id: 336, list_revision: 10, article_revision: 2, retrieved_at: "2026-11-10T12:00:00Z" },
    corrections: { name: { protected: true, value: "My event", actor: "user:test", at: "2026-11-10T12:00:00Z" }, date: { protected: true, value: null, actor: "user:test", at: "2026-11-10T12:00:00Z" } },
    aliases: [], review: []
  };
}

function setup(event = fixture()) {
  const read = vi.spyOn(APIClient.ufc, "event").mockResolvedValue(event);
  const save = vi.spyOn(APIClient.ufc, "editEvent").mockResolvedValue({ ...event, revision: event.revision + 1 });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><UFCEventEditor id={event.id} onClose={vi.fn()} /></QueryClientProvider>);
  return { read, save, client };
}

test("alias ordering does not warn or swap rows when applying the default", async () => {
  const event = fixture();
  const title = { namespace: "title", value: "Wikipedia event", current: true, verified: true };
  const number = { namespace: "ufc.numbered", value: "336", current: true, verified: true };
  event.aliases = [title, number];
  event.source.aliases = [number, title];
  const { save } = setup(event);
  const aliases = within(await screen.findByRole("region", { name: "Event aliases" }));
  expect(aliases.queryByText(/^Wikipedia suggestion:/)).toBeNull();
  fireEvent.click(aliases.getByText("Edit Aliases"));
  const before = [1, 2].map(index => (aliases.getByLabelText(`Alias ${index} value`) as HTMLInputElement).value);
  fireEvent.click(aliases.getByRole("button", { name: "Use Default" }));
  expect([1, 2].map(index => (aliases.getByLabelText(`Alias ${index} value`) as HTMLInputElement).value)).toEqual(before);
  expect(save).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Save Corrections" }));
  await waitFor(() => expect(save).toHaveBeenCalledWith(event.id, { revision: 2, aliases: { operation: "reset" } }));
});

test("resolution choices are always visible and editing creates a per-event override", async () => {
  const event = { ...fixture(), quality: { defaults: ["1080p"], override: null, effective: ["1080p"] } };
  const { save } = setup(event);
  const group = within(await screen.findByRole("group", { name: "Accepted Resolutions" }));
  expect(group.getAllByRole("switch")).toHaveLength(5);
  expect((group.getByRole("switch", { name: "1080p" }) as HTMLElement).getAttribute("aria-checked")).toBe("true");
  expect((group.getByRole("switch", { name: "720p" }) as HTMLElement).getAttribute("aria-checked")).toBe("false");
  fireEvent.click(group.getByRole("switch", { name: "720p" }));
  fireEvent.click(screen.getByRole("button", { name: "Save Corrections" }));
  await waitFor(() => expect(save).toHaveBeenCalledWith(event.id, { revision: 2, quality: { mode: "custom", resolutions: ["1080p", "720p"] } }));
});

test("resolution choices immediately restore Settings values when removing an override", async () => {
  const event = { ...fixture(), quality: { defaults: ["1080p"], override: ["720p"], effective: ["720p"] } };
  const { save } = setup(event);
  const group = within(await screen.findByRole("group", { name: "Accepted Resolutions" }));
  fireEvent.click(group.getByRole("button", { name: /Use Settings defaults/ }));
  expect((group.getByRole("switch", { name: "1080p" }) as HTMLElement).getAttribute("aria-checked")).toBe("true");
  expect((group.getByRole("switch", { name: "720p" }) as HTMLElement).getAttribute("aria-checked")).toBe("false");
  fireEvent.click(screen.getByRole("button", { name: "Save Corrections" }));
  await waitFor(() => expect(save).toHaveBeenCalledWith(event.id, { revision: 2, quality: { mode: "inherit" } }));
});

test("shows protected fields and differing source suggestions, saves a protected clear without resetting", async () => {
  const { save } = setup();
  await screen.findByLabelText("Event Name");
  expect(screen.getAllByText("Protected")).toHaveLength(2);
  expect(screen.getByText("Wikipedia suggestion: Wikipedia event")).toBeTruthy();
  expect(screen.getByText("Wikipedia suggestion: 2027-01-30")).toBeTruthy();
  fireEvent.change(screen.getByLabelText("Event date"), { target: { value: "2027-02-01" } });
  fireEvent.change(screen.getByLabelText("Event date"), { target: { value: "" } });
  fireEvent.change(screen.getByLabelText("First Headliner"), { target: { value: "" } });
  fireEvent.change(screen.getByLabelText("Second Headliner"), { target: { value: "" } });
  fireEvent.click(screen.getByRole("button", { name: "Save Corrections" }));
  await waitFor(() => expect(save).toHaveBeenCalledWith("stable-local", { revision: 2, date: { operation: "set", value: null }, headliners: { operation: "set", names: [] } }));
  await screen.findByText("Event saved.");
});

test("stages per-field Wikipedia reset separately from clearing and leaves other fields unchanged", async () => {
  const original = fixture();
  const { save } = setup(original);
  save.mockResolvedValue({ ...original, revision: 3, values: { ...original.values, name: original.source.name }, corrections: { date: original.corrections.date } });
  await screen.findByLabelText("Event Name");
  fireEvent.click(screen.getByRole("button", { name: "Use default for Name" }));
  expect((screen.getByLabelText("Event Name") as HTMLInputElement).value).toBe("Wikipedia event");
  expect(screen.getByText("Default (on save)")).toBeTruthy();
  expect(save).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Save Corrections" }));
  await waitFor(() => expect(save).toHaveBeenCalledWith("stable-local", { revision: 2, name: { operation: "reset" } }));
  await screen.findByText("Event saved.");
  expect(screen.getAllByText("Protected")).toHaveLength(1);
});

test("saves an edited name, known date, ordered headliner pair and status", async () => {
  const { save } = setup();
  await screen.findByLabelText("Event Name");
  fireEvent.change(screen.getByLabelText("Event Name"), { target: { value: "Corrected event" } });
  fireEvent.change(screen.getByLabelText("Event date"), { target: { value: "2027-02-06" } });
  fireEvent.change(screen.getByLabelText("First Headliner"), { target: { value: "New First" } });
  fireEvent.change(screen.getByLabelText("Second Headliner"), { target: { value: "New Second" } });
  fireEvent.change(screen.getByLabelText("Event status"), { target: { value: "completed" } });
  fireEvent.click(screen.getByRole("button", { name: "Save Corrections" }));
  await waitFor(() => expect(save).toHaveBeenCalledWith("stable-local", {
    revision: 2, name: { operation: "set", value: "Corrected event" }, date: { operation: "set", value: "2027-02-06" },
    headliners: { operation: "set", names: ["New First", "New Second"] }, status: { operation: "set", value: "completed" }
  }));
});

test("keeps unsaved inputs on stale save and requires explicit latest-revision review", async () => {
  const original = fixture();
  const { read, save } = setup(original);
  save.mockRejectedValueOnce(Object.assign(new Error("Revision conflict"), { status: 409 }));
  await screen.findByLabelText("Event Name");
  fireEvent.change(screen.getByLabelText("Event Name"), { target: { value: "My unsaved draft" } });
  fireEvent.click(screen.getByRole("button", { name: "Save Corrections" }));
  await screen.findByText(/This event has a newer revision/);
  expect((screen.getByLabelText("Event Name") as HTMLInputElement).value).toBe("My unsaved draft");
  expect((screen.getByRole("button", { name: "Save Corrections" }) as HTMLButtonElement).disabled).toBe(true);
  read.mockResolvedValue({ ...original, revision: 4, values: { ...original.values, name: "Someone else's edit" } });
  fireEvent.click(screen.getByRole("button", { name: "Review latest revision" }));
  await screen.findByText("Latest saved revision: 4");
  expect(screen.getByText(/Name: Someone else's edit/)).toBeTruthy();
  expect(save).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "Keep My Draft Against This Revision" }));
  expect((screen.getByLabelText("Event Name") as HTMLInputElement).value).toBe("My unsaved draft");
  fireEvent.click(screen.getByRole("button", { name: "Save Corrections" }));
  await waitFor(() => expect(save).toHaveBeenLastCalledWith("stable-local", { revision: 4, name: { operation: "set", value: "My unsaved draft" } }));
});

test("a refreshed detail updates source suggestions without discarding unsaved inputs", async () => {
  const original = fixture();
  const { read, client } = setup(original);
  await screen.findByLabelText("Event Name");
  fireEvent.change(screen.getByLabelText("Event Name"), { target: { value: "Unsaved name" } });
  read.mockResolvedValue({ ...original, revision: 3, source: { ...original.source, name: "New source name" } });
  await act(async () => { await client.refetchQueries({ queryKey: UFCEventQueryOptions(original.id).queryKey }); });
  await screen.findByText("Wikipedia suggestion: New source name");
  expect((screen.getByLabelText("Event Name") as HTMLInputElement).value).toBe("Unsaved name");
});

test("handles validation errors without discarding a draft", async () => {
  const { save } = setup();
  save.mockRejectedValue(new Error("headliners must be unknown or an ordered pair"));
  await screen.findByLabelText("First Headliner");
  fireEvent.change(screen.getByLabelText("First Headliner"), { target: { value: "Only one" } });
  fireEvent.change(screen.getByLabelText("Second Headliner"), { target: { value: "" } });
  fireEvent.click(screen.getByRole("button", { name: "Save Corrections" }));
  await screen.findByRole("alert");
  expect((screen.getByLabelText("First Headliner") as HTMLInputElement).value).toBe("Only one");
});

test("Events opens the editor and refreshes details while retaining the draft", async () => {
  const original = fixture();
  vi.spyOn(APIClient.ufc, "events").mockResolvedValue({ upcoming: [{ id: original.id, name: original.values.name, date: null, aliases: [], needs_review: false, review: [], source_url: "https://en.wikipedia.org/?curid=336" }], recent_completed: [] });
  vi.spyOn(APIClient.ufc, "catalogueStatus").mockResolvedValue({ running: false, interrupted: false, last_success: null, last_error: "", report: { diagnostics: [], diagnostics_omitted: 0 } });
  const read = vi.spyOn(APIClient.ufc, "event").mockResolvedValue(original);
  vi.spyOn(APIClient.ufc, "refreshCatalogue").mockResolvedValue({ coalesced: false });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><UFCEvents /></QueryClientProvider>);
  fireEvent.click(await screen.findByRole("button", { name: "Edit My event" }));
  const editor = within(await screen.findByRole("region", { name: "Event editor" }));
  fireEvent.change(await editor.findByLabelText("Event Name"), { target: { value: "Draft during refresh" } });
  read.mockResolvedValue({ ...original, revision: 3, source: { ...original.source, name: "Fresh source suggestion" } });
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await screen.findByText("Wikipedia suggestion: Fresh source suggestion");
  expect((editor.getByLabelText("Event Name") as HTMLInputElement).value).toBe("Draft during refresh");
});

test("previews per-event component choices and saves only changed overrides", async () => {
  const original = fixture();
  original.components = { defaults: ["main_card", "prelims", "early_prelims"], overrides: { prelims: false, embedded: true }, wanted: ["main_card", "early_prelims", "embedded"] };
  const { save } = setup(original);
  await screen.findByLabelText("Prelims");
  expect((screen.getByLabelText("Prelims") as HTMLSelectElement).value).toBe("off");
  expect((screen.getByLabelText("Main Card") as HTMLSelectElement).value).toBe("inherit");
  fireEvent.change(screen.getByLabelText("Prelims"), { target: { value: "inherit" } });
  fireEvent.change(screen.getByLabelText("Countdown"), { target: { value: "on" } });
  expect(screen.getByText("Wanted on save: Main Card, Prelims, Early Prelims, Countdown, Embedded")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Save Corrections" }));
  await waitFor(() => expect(save).toHaveBeenCalledWith("stable-local", { revision: 2, components: { prelims: "inherit", countdown: "on" } }));
});

test("retains component choices on conflict while showing the latest saved preferences", async () => {
  const original = fixture();
  original.components = { defaults: ["main_card", "prelims"], overrides: {}, wanted: ["main_card", "prelims"] };
  const { save, read } = setup(original);
  save.mockRejectedValueOnce(Object.assign(new Error("Conflict"), { status: 409 }));
  await screen.findByLabelText("Embedded");
  fireEvent.change(screen.getByLabelText("Embedded"), { target: { value: "on" } });
  fireEvent.click(screen.getByRole("button", { name: "Save Corrections" }));
  await screen.findByText(/This event has a newer revision/);
  read.mockResolvedValue({ ...original, revision: 4, components: { ...original.components, overrides: { prelims: false }, wanted: ["main_card"] } });
  fireEvent.click(screen.getByRole("button", { name: "Review latest revision" }));
  await screen.findByText("Latest wanted components: Main Card");
  expect((screen.getByLabelText("Embedded") as HTMLSelectElement).value).toBe("on");
  fireEvent.click(screen.getByRole("button", { name: "Keep My Draft Against This Revision" }));
  fireEvent.click(screen.getByRole("button", { name: "Save Corrections" }));
  await waitFor(() => expect(save).toHaveBeenLastCalledWith("stable-local", { revision: 4, components: { embedded: "on" } }));
});
