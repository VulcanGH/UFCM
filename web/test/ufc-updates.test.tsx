import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { APIClient } from "@api/APIClient";
import { UFCUpdates } from "@screens/UFCUpdates";
import type { UFCUpdateStatus } from "@app/types/UFCUpdate";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });
const fixture = (): UFCUpdateStatus => ({revision: 4, auto_install: false, id: "verified-release", version: "v1.0.1", phase: "verified", message: "Ready to update", current: "1.0.0", supported: true, reason: "", release_url: "https://github.com/VulcanGH/UFCM/releases"});
function setup(value: UFCUpdateStatus) {
  vi.spyOn(APIClient.ufc, "updateStatus").mockResolvedValue(value);
  const client = new QueryClient({defaultOptions: {queries: {retry: false}, mutations: {retry: false}}});
  render(<QueryClientProvider client={client}><UFCUpdates /></QueryClientProvider>);
}

test("requests only the verified ID with one click and preserves automatic-install revision", async () => {
  const install = vi.spyOn(APIClient.ufc, "installUpdate").mockResolvedValue({...fixture(), phase: "waiting_for_idle"});
  const preferences = vi.spyOn(APIClient.ufc, "saveUpdatePreferences").mockResolvedValue({...fixture(), revision: 5, auto_install: true});
  setup(fixture());
  fireEvent.click(await screen.findByRole("switch", {name: "Automatically Install When Idle"}));
  await waitFor(() => expect(preferences).toHaveBeenCalledWith(4, true));
  fireEvent.click(screen.getByRole("button", {name: "Update and Restart"}));
  await waitFor(() => expect(install).toHaveBeenCalledWith("verified-release", expect.anything()));
  await screen.findByText("Waiting for Current Work");
  expect(screen.queryByRole("dialog")).toBeNull();
});

test("PostgreSQL shows manual guidance without installation or automatic controls", async () => {
  setup({...fixture(), supported: false, reason: "PostgreSQL updates are installed manually"});
  await screen.findByText("PostgreSQL updates are installed manually");
  expect(screen.queryByRole("button", {name: "Update and Restart"})).toBeNull();
  expect(screen.queryByRole("switch")).toBeNull();
  expect(screen.getByRole("link", {name: "Release Notes"}).getAttribute("href")).toBe(fixture().release_url);
});

test("checking gives immediate feedback and displays the returned outcome", async () => {
  let finish!: (value: UFCUpdateStatus) => void;
  vi.spyOn(APIClient.ufc, "checkUpdate").mockImplementation(() => new Promise(resolve => { finish = resolve; }));
  setup({...fixture(), phase: "idle", message: ""});
  fireEvent.click(await screen.findByRole("button", {name: "Check for Updates"}));
  expect((await screen.findByRole("button", {name: "Checking…"}) as HTMLButtonElement).disabled).toBe(true);
  vi.mocked(APIClient.ufc.updateStatus).mockResolvedValue({...fixture(), phase: "no_release", message: "No public stable release is available yet"});
  finish({...fixture(), phase: "no_release", message: "No public stable release is available yet"});
  await screen.findByText("No Public Release Yet");
});

test("check errors are visible", async () => {
  vi.spyOn(APIClient.ufc, "checkUpdate").mockRejectedValue(new Error("Release signing key is not configured"));
  setup({...fixture(), phase: "idle", message: ""});
  fireEvent.click(await screen.findByRole("button", {name: "Check for Updates"}));
  expect((await screen.findByRole("alert")).textContent).toContain("Release signing key is not configured");
});
