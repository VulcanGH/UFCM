import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";

import { APIClient } from "@api/APIClient";
import type { UFCCatalogueStatus, UFCEventList, UFCEventSummary } from "@app/types/UFC";
import { UFCEvents } from "@screens/UFCEvents";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

test("past events loads twenty initially and all history on request", async () => {
  vi.spyOn(APIClient.ufc, "catalogueStatus").mockResolvedValue(idle);
  vi.spyOn(APIClient.ufc, "libraryCoverage").mockResolvedValue({configured:false,running:false,unavailable:false,review:0,events:{}});
  const listing = vi.spyOn(APIClient.ufc, "catalogueEvents").mockImplementation(async (_search, _offset, limit = 20) => ({ total: 45, events: Array.from({length: Math.min(limit,45)}, (_, i) => event(`Historical ${i}`, "2024-01-01")) }));
  renderEvents(true);
  await screen.findByText("Historical 19");
  expect(screen.queryByText("Historical 20")).toBeNull();
  expect(listing).toHaveBeenCalledWith("",0,20);
  fireEvent.click(screen.getByRole("button",{name:"Load All Events"}));
  await screen.findByText("Historical 44");
  expect(listing).toHaveBeenCalledWith("",0,2500);
  expect(screen.queryByRole("button",{name:"Load All Events"})).toBeNull();
});

const event = (name: string, date: string | null, id = name): UFCEventSummary => ({
  id, name, date, aliases: [], needs_review: false, review: [], source_url: "https://en.wikipedia.org/?curid=123"
});
const idle: UFCCatalogueStatus = {
  running: false, interrupted: false, last_success: null, last_error: "", report: { diagnostics: [], diagnostics_omitted: 0 }
};

function renderEvents(past = false) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const rendered = render(<QueryClientProvider client={client}><UFCEvents past={past} /></QueryClientProvider>);
  return { ...rendered, client };
}

test("shows upcoming only, retaining aliases and review indicators", async () => {
  const upcoming = event("Procházka vs. Stirling", "2026-11-21");
  upcoming.needs_review = true; upcoming.review = ["competing_event_identifier"];
  upcoming.aliases = [
    { namespace: "ufc.fight_night", value: "294", current: true, verified: true },
    { namespace: "title", value: "Earlier billing", current: false, verified: false }
  ];
  vi.spyOn(APIClient.ufc, "events").mockResolvedValue({ upcoming: [upcoming, event("Future TBA", null)], recent_completed: Array.from({ length: 10 }, (_, i) => event(`Completed ${10 - i}`, `2026-10-${String(20 - i).padStart(2, "0")}`)) });
  vi.spyOn(APIClient.ufc, "catalogueStatus").mockResolvedValue(idle);
  renderEvents();
  await screen.findByText("Procházka vs. Stirling");
  expect(screen.getByText("Needs Review").title).toContain("competing event identifier");
  expect(screen.getByText(/UFC Fight Night 294/).textContent).toContain("Earlier billing (former)");
  expect(screen.getByText(/Date to be announced/)).toBeTruthy();
  expect(screen.getByRole("link", { name: "Procházka vs. Stirling" }).getAttribute("href")).toBe(upcoming.source_url);
  expect(screen.queryByRole("link", { name: "Wikipedia source" })).toBeNull();
  expect(within(screen.getByRole("region", { name: "Upcoming Events" })).getAllByRole("heading", { level: 4 }).map(h => h.textContent)).toEqual(["Procházka vs. Stirling", "Future TBA"]);
  expect(screen.queryByRole("region", { name: "Recent completed events" })).toBeNull();
  expect(screen.getByText("2026-11-21").getAttribute("datetime")).toBe("2026-11-21");
});

test("refresh polls through completion and reloads saved events", async () => {
  const listing = vi.spyOn(APIClient.ufc, "events").mockResolvedValue({ upcoming: [], recent_completed: [] });
  let running = false;
  vi.spyOn(APIClient.ufc, "catalogueStatus").mockImplementation(async () => ({ ...idle, running }));
  const refresh = vi.spyOn(APIClient.ufc, "refreshCatalogue").mockImplementation(async () => { running = true; return { coalesced: false }; });
  renderEvents();
  await screen.findByText(/No upcoming events saved/);
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await waitFor(() => expect(refresh).toHaveBeenCalledTimes(1));
  await waitFor(() => expect((screen.getByRole("button", { name: "Refreshing…" }) as HTMLButtonElement).disabled).toBe(true));
  listing.mockResolvedValue({ upcoming: [event("New event", "2027-01-30")], recent_completed: [] });
  running = false;
  await screen.findByText("New event", {}, { timeout: 3000 });
  expect((screen.getByRole("button", { name: "Refresh" }) as HTMLButtonElement).disabled).toBe(false);
  expect(listing.mock.calls.length).toBeGreaterThan(1);
});

test("retries status polling after a transient error following refresh", async () => {
  const listing = vi.spyOn(APIClient.ufc, "events").mockResolvedValue({ upcoming: [], recent_completed: [] });
  let accepted = false;
  let reads = 0;
  vi.spyOn(APIClient.ufc, "catalogueStatus").mockImplementation(async () => {
    if (accepted && reads++ === 0) throw new Error("Temporary status failure");
    return idle;
  });
  vi.spyOn(APIClient.ufc, "refreshCatalogue").mockImplementation(async () => {
    accepted = true;
    listing.mockResolvedValue({ upcoming: [event("Recovered refresh", "2027-01-30")], recent_completed: [] });
    return { coalesced: false };
  });
  renderEvents(); await screen.findByText(/No upcoming events saved/);
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await screen.findByText("Recovered refresh", {}, { timeout: 3000 });
  expect(screen.queryByText(/Refresh status is unavailable/)).toBeNull();
});

test("handles a refresh completed before the first status read", async () => {
  const listing = vi.spyOn(APIClient.ufc, "events").mockResolvedValue({ upcoming: [], recent_completed: [] });
  vi.spyOn(APIClient.ufc, "catalogueStatus").mockResolvedValue(idle);
  vi.spyOn(APIClient.ufc, "refreshCatalogue").mockImplementation(async () => {
    listing.mockResolvedValue({ upcoming: [event("Fast refresh", "2027-01-30")], recent_completed: [] });
    return { coalesced: true };
  });
  renderEvents(); await screen.findByText(/No upcoming events saved/);
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await screen.findByText("Fast refresh");
});

test("keeps saved events visible when refresh fails", async () => {
  vi.spyOn(APIClient.ufc, "events").mockResolvedValue({ upcoming: [event("Saved event", "2027-01-30")], recent_completed: [] });
  vi.spyOn(APIClient.ufc, "catalogueStatus").mockResolvedValue(idle);
  vi.spyOn(APIClient.ufc, "refreshCatalogue").mockRejectedValue(new Error("Connection unavailable"));
  renderEvents(); await screen.findByText("Saved event");
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await screen.findByText(/Refresh could not be started/);
  expect(screen.getByText("Saved event")).toBeTruthy();
});

test("shows backend refresh failure and resumes a refresh already running", async () => {
  const saved: UFCEventList = { upcoming: [], recent_completed: [] };
  vi.spyOn(APIClient.ufc, "events").mockResolvedValue(saved);
  const status = vi.spyOn(APIClient.ufc, "catalogueStatus").mockResolvedValue({ ...idle, running: true });
  const { client } = renderEvents();
  await screen.findByRole("button", { name: "Refreshing…" });
  status.mockResolvedValue({ ...idle, last_error: "Source unavailable" });
  await act(async () => { await client.refetchQueries({ queryKey: ["ufc", "catalogue-status"] }); });
  await screen.findByText(/Refresh failed. Saved events remain available/);
  expect(screen.getByRole("button", { name: "Refresh" })).toBeTruthy();
});

test("puts last refreshed after the upcoming list and limits the review summary to visible upcoming events", async () => {
  const upcoming = { ...event("Upcoming review", "2027-01-30"), needs_review: true, review: ["upcoming_issue"] };
  const completed = { ...event("Completed review", "2026-11-01"), needs_review: true, review: ["historical_issue"] };
  vi.spyOn(APIClient.ufc, "events").mockResolvedValue({ upcoming: [upcoming], recent_completed: [completed] });
  vi.spyOn(APIClient.ufc, "catalogueStatus").mockResolvedValue({ ...idle, last_success: "2026-11-10T10:00:00Z", report: { diagnostics: [{ name: "Old unresolved row", reason: "old_issue" }], diagnostics_omitted: 5 } });
  renderEvents();
  await screen.findByText("Upcoming review");
  const upcomingGroup = screen.getByRole("region", { name: "Upcoming Events" });
  const refreshed = screen.getByText(/Last refreshed:/);
  expect(upcomingGroup.compareDocumentPosition(refreshed) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  const summary = screen.getByText("Refresh entries needing review (1)");
  fireEvent.click(summary);
  const details = summary.closest("details")!;
  expect(within(details).getByText("Upcoming review: upcoming issue")).toBeTruthy();
  expect(within(details).queryByText(/historical issue|Old unresolved row/)).toBeNull();
  expect(screen.queryByText(/Event data:/)).toBeNull();
  expect(screen.queryByRole("link", { name: "CC BY-SA 4.0" })).toBeNull();
});

test("hides the review section when only completed or unresolved historical entries need review", async () => {
  vi.spyOn(APIClient.ufc, "events").mockResolvedValue({ upcoming: [event("Upcoming ready", "2027-01-30")], recent_completed: [{ ...event("Completed review", "2026-11-01"), needs_review: true, review: ["historical_issue"] }] });
  vi.spyOn(APIClient.ufc, "catalogueStatus").mockResolvedValue({ ...idle, report: { diagnostics: [{ name: "Old unresolved row", reason: "old_issue" }], diagnostics_omitted: 0 } });
  renderEvents();
  await screen.findByText("Upcoming ready");
  expect(screen.queryByText(/Refresh entries needing review/)).toBeNull();
  expect(screen.queryByText("Needs Review")).toBeNull();
});
