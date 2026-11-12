import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { createMemoryHistory, createRootRoute, createRoute, createRouter, Outlet, RouterProvider } from "@tanstack/react-router";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";

import { APIClient } from "@api/APIClient";
import { UFCLayout } from "@screens/UFCLayout";
import { UFCEvents } from "@screens/UFCEvents";
import { UFCSettings } from "@screens/UFCSettings";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

test("UFC sidebar switches between Events and Settings with existing status", async () => {
  vi.spyOn(APIClient.ufc, "settings").mockResolvedValue({ revision: 1, templates: { numbered: ["main_card", "prelims", "early_prelims"], fight_night: ["main_card", "prelims"] }, event_counts: { numbered: 0, fight_night: 0 } });
  vi.spyOn(APIClient.ufc, "events").mockResolvedValue({ upcoming: [], recent_completed: [] });
  vi.spyOn(APIClient.ufc, "catalogueStatus").mockResolvedValue({ running: false, interrupted: false, last_success: null, last_error: "", report: { diagnostics: [], diagnostics_omitted: 0 } });
  vi.spyOn(APIClient.ufc, "status").mockResolvedValue({ product_slug: "ufc-manager", product_name: "UFC Manager", schema_ready: true, settings_revision: 1, automatic_acquisition_enabled: false, processing_enabled: false });
  const root = createRootRoute({ component: Outlet });
  const ufc = createRoute({ getParentRoute: () => root, path: "ufc", component: UFCLayout });
  const events = createRoute({ getParentRoute: () => ufc, path: "events", component: UFCEvents });
  const manager = createRoute({ getParentRoute: () => ufc, path: "settings", component: UFCSettings });
  const router = createRouter({ routeTree: root.addChildren([ufc.addChildren([events, manager])]), history: createMemoryHistory({ initialEntries: ["/ufc/events"] }) });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  await act(async () => { render(<QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider>); });
  await screen.findByText(/No upcoming events saved/);
  expect(router.state.location.pathname).toBe("/ufc/events");
  const sidebar = screen.getByRole("navigation", { name: "UFC navigation" });
  expect(within(sidebar).getAllByRole("link").map(link => link.textContent)).toEqual(["Upcoming Events", "Past Events", "Library", "Downloads", "Testing", "Settings"]);
  expect(within(sidebar).getByRole("link", { name: "Upcoming Events" }).getAttribute("aria-current")).toBe("page");
  fireEvent.click(within(sidebar).getByRole("link", { name: "Settings" }));
  await screen.findByText("Application Ready");
  expect(router.state.location.pathname).toBe("/ufc/settings");
  expect(screen.getByRole("group", { name: "Automation Settings" })).toBeTruthy();
  expect(within(sidebar).getByRole("link", { name: "Settings" }).getAttribute("aria-current")).toBe("page");
  fireEvent.click(within(sidebar).getByRole("link", { name: "Upcoming Events" }));
  await screen.findByText(/No upcoming events saved/);
  expect(router.state.location.pathname).toBe("/ufc/events");
});
