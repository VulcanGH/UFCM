import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { ReactNode } from "react";
import { APIClient } from "@api/APIClient";
import { UFCLibrary } from "@screens/UFCLibrary";

vi.mock("@tanstack/react-router", () => ({ Link: ({ to, children }: { to: string; children: ReactNode }) => <a href={to}>{children}</a> }));
afterEach(() => { cleanup(); vi.restoreAllMocks(); });

test("library review confirms identity without asking for missing resolution", async () => {
  vi.spyOn(APIClient.ufc, "library").mockResolvedValue({ configured: true, settings: { revision: 1, running: false, run_id: "", last_scan: null, message: "", roots: [] }, groups: [{ id: "event:one", event_id: "one", name: "UFC 100", date: null, files: 1, review: 1, unavailable: 0, extras: 0, locations: 1, components: {} }] });
  const file = { id: "file", revision: 1, root_id: "root", root_path: "fixture", path: "UFC 100.mp4", size: 100, modified: "", event_id: "one", component: "main_card", resolution: "unknown", state: "review", reason: "Review event identity", manual: false, present: true, available: true, group: "event:one", detected_event: "one", detected_component: "main_card", detected_resolution: "unknown", detected_state: "review", detected_reason: "" };
  vi.spyOn(APIClient.ufc, "libraryFiles").mockResolvedValue({ total: 1, files: [file] });
  vi.spyOn(APIClient.ufc, "libraryEvents").mockResolvedValue([{ id: "one", name: "UFC 100", date: null }, { id: "old", name: "UFC 2", date: "1994-03-11" }, { id: "new", name: "UFC 300", date: "2024-04-13" }]);
  const save = vi.spyOn(APIClient.ufc, "editLibraryFile").mockResolvedValue({ ...file, manual: true, state: "confirmed" });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><UFCLibrary /></QueryClientProvider>);
  fireEvent.click(await screen.findByRole("button", { name: "Show Files" }));
  await screen.findByText(/Main Card · review/);
  expect(screen.queryByText(/Main Card · unknown/)).toBeNull();
  fireEvent.click(await screen.findByRole("button", { name: "Review" }));
  await screen.findByLabelText("Event");
  await screen.findByRole("option", { name: "UFC 300 (2024-04-13)" });
  const picker = screen.getByLabelText("Event") as HTMLSelectElement;
  expect(Array.from(picker.options).map(option => option.value)).toEqual(["", "new", "old", "one"]);
  fireEvent.change(screen.getByLabelText("Find Event"), { target: { value: "UFC 300 2024" } });
  expect(Array.from(picker.options).map(option => option.value)).toEqual(["", "new", "one"]);
  expect(picker.value).toBe("one");
  expect(picker.size).toBe(6);
  expect(screen.getByText(/1 matching event\. Select an event above/)).toBeTruthy();
  fireEvent.change(screen.getByLabelText("Find Event"), { target: { value: "no such event" } });
  expect(screen.getByText(/No matching events\./)).toBeTruthy();
  expect(picker.value).toBe("one");
  fireEvent.change(screen.getByLabelText("Find Event"), { target: { value: "UFC 300" } });
  fireEvent.change(picker, { target: { value: "new" } });
  expect(screen.queryByLabelText("Resolution")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Confirm identification" }));
  await vi.waitFor(() => expect(save).toHaveBeenCalledWith("file", { revision: 1, mode: "confirm", event_id: "new", component: "main_card", resolution: "unknown" }));
});

test("library pagination exposes every group beyond ten pages", async () => {
  const groups = Array.from({ length: 601 }, (_, i) => ({ id: `group-${i}`, event_id: "", name: `Event ${i}`, date: null, files: 1, review: 0, unavailable: 0, extras: 0, locations: 1, wanted: [], components: {} }));
  vi.spyOn(APIClient.ufc, "library").mockResolvedValue({ configured: true, settings: { revision: 1, running: false, run_id: "", last_scan: null, message: "", roots: [] }, groups });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><UFCLibrary /></QueryClientProvider>);
  await screen.findByText("601 event/folder groups");
  for (let page = 1; page <= 13; page++) {
    expect(screen.getByText(`Page ${page} of 13`)).toBeTruthy();
    expect(screen.getByRole("heading", { name: `Event ${(page - 1) * 50}` })).toBeTruthy();
    if (page < 13) fireEvent.click(screen.getByRole("button", { name: "Next" }));
  }
  expect((screen.getByRole("button", { name: "Next" }) as HTMLButtonElement).disabled).toBe(true);
  fireEvent.change(screen.getByLabelText("Find library event"), { target: { value: "Event 600" } });
  expect(screen.getByRole("heading", { name: "Event 600" })).toBeTruthy();
});

test("keeps library rows compact and expands files only on request", async () => {
  vi.spyOn(APIClient.ufc, "library").mockResolvedValue({ configured: true, settings: { revision: 1, running: false, run_id: "", last_scan: null, message: "", roots: [{ id: "root", path: "X:/fixture", enabled: true, state: "available", message: "" }] }, groups: [{ id: "event:one", event_id: "one", name: "UFC 173 Barao vs. Dillashaw", date: "2014-05-24", files: 3, review: 0, unavailable: 0, extras: 0, locations: 1, wanted: ["main_card", "prelims"], components: { main_card: "present", prelims: "present", early_prelims: "present" } }] });
  const files = vi.spyOn(APIClient.ufc, "libraryFiles").mockResolvedValue({ total: 0, files: [] });
  const scan = vi.spyOn(APIClient.ufc, "scanLibrary").mockResolvedValue({ run_id: "fixture" });
  const clear = vi.spyOn(APIClient.ufc, "clearLibrary").mockResolvedValue({ cleared: true });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><UFCLibrary /></QueryClientProvider>);
  await screen.findByRole("heading", { name: /UFC 173/ });
  expect(files).not.toHaveBeenCalled();
  expect(screen.getByTitle("Prelims: present")).toBeTruthy();
  expect(screen.queryByTitle(/Early Prelims:/)).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Show Files" }));
  await screen.findByRole("button", { name: "Hide Files" });
  expect(files).toHaveBeenCalledWith("event:one", 0);
  fireEvent.click(screen.getByRole("button", { name: "Scan Library" }));
  await vi.waitFor(() => expect(scan).toHaveBeenCalledTimes(1));
  fireEvent.change(screen.getByLabelText("Library filter"), { target: { value: "review" } });
  expect(screen.queryByRole("heading", { name: /UFC 173/ })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "Clear Library" }));
  expect(clear).not.toHaveBeenCalled();
  expect(screen.getByText(/Files on disk, configured folders and the event catalogue are kept/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Confirm Clear Library" }));
  await vi.waitFor(() => expect(clear).toHaveBeenCalledWith(1));
});
