import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";

import { APIClient } from "@api/APIClient";
import type { UFCSettings as Settings } from "@app/types/UFC";
import { UFCSettings } from "@screens/UFCSettings";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

const fixture = (): Settings => ({ revision: 3, templates: { numbered: ["main_card", "prelims", "early_prelims"], fight_night: ["main_card", "prelims"] }, event_counts: { numbered: 8, fight_night: 10 } });

function setup() {
  vi.spyOn(APIClient.ufc,"mainCardTerms").mockResolvedValue([{name:"Defaults",terms:["pre show","press conference"]}]);
  const read = vi.spyOn(APIClient.ufc, "settings").mockResolvedValue(fixture());
  const save = vi.spyOn(APIClient.ufc, "saveSettings").mockImplementation(async edit => ({ ...fixture(), revision: 4, templates: edit.templates }));
  vi.spyOn(APIClient.ufc, "status").mockResolvedValue({ product_slug: "ufc-manager", product_name: "UFC Manager", schema_ready: true, settings_revision: 1, automatic_acquisition_enabled: false, processing_enabled: false });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><UFCSettings /></QueryClientProvider>);
  return { read, save };
}

test("shows both seeded defaults, previews changes and saves with the settings revision", async () => {
  const { save } = setup();
  const numbered = within(await screen.findByRole("group", { name: "Numbered events" }));
  const fightNight = within(screen.getByRole("group", { name: "Fight Nights" }));
  expect(numbered.getByRole("switch", { name: "Early Prelims" }).getAttribute("aria-checked")).toBe("true");
  expect(fightNight.getByRole("switch", { name: "Early Prelims" }).getAttribute("aria-checked")).toBe("false");
  fireEvent.click(numbered.getByRole("switch", { name: "Countdown" }));
  expect(screen.getByRole("region", { name: "Default changes preview" }).textContent).toContain("8 existing events; only inherited choices change");
  expect(screen.getByText("Explicit event overrides will be retained.")).toBeTruthy();
  expect(save).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: /save settings/i }));
  await waitFor(() => expect(save).toHaveBeenCalledWith({ revision: 3, templates: { numbered: ["main_card", "prelims", "early_prelims", "countdown"], fight_night: ["main_card", "prelims"] } }));
  await screen.findByText("Acquisition defaults saved.");
  expect(within(screen.getByRole("group", {name: "Automation Settings"})).getAllByText("Unavailable").length).toBeGreaterThan(0);
});

test("retains a stale draft for explicit revision review and preserves untouched event-type defaults", async () => {
  const { read, save } = setup();
  save.mockRejectedValueOnce(Object.assign(new Error("Conflict"), { status: 409 }));
  const numbered = within(await screen.findByRole("group", { name: "Numbered events" }));
  fireEvent.click(numbered.getByRole("switch", { name: "Countdown" }));
  fireEvent.click(screen.getByRole("button", { name: /save settings/i }));
  await screen.findByText(/Settings changed since this form was loaded/);
  expect(numbered.getByRole("switch", { name: "Countdown" }).getAttribute("aria-checked")).toBe("true");
  expect((screen.getByRole("button", { name: /save settings/i }) as HTMLButtonElement).disabled).toBe(true);
  read.mockResolvedValue({ ...fixture(), revision: 5, templates: { ...fixture().templates, fight_night: ["main_card", "embedded"] } });
  fireEvent.click(screen.getByRole("button", { name: "Review latest settings" }));
  await screen.findByText("Latest Fight Nights: Main Card, Embedded");
  expect(save).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByRole("button", { name: "Keep my choices against this revision" }));
  fireEvent.click(screen.getByRole("button", { name: /save settings/i }));
  await waitFor(() => expect(save).toHaveBeenLastCalledWith({ revision: 5, templates: { numbered: ["main_card", "prelims", "early_prelims", "countdown"], fight_night: ["main_card", "embedded"] } }));
});

test("keeps choices after a failed save", async () => {
  const { save } = setup();
  save.mockRejectedValue(new Error("Temporary database failure"));
  const numbered = within(await screen.findByRole("group", { name: "Numbered events" }));
  fireEvent.click(numbered.getByRole("switch", { name: "Weigh-ins" }));
  fireEvent.click(screen.getByRole("button", { name: /save settings/i }));
  await screen.findByText(/Could not save defaults/);
  expect(numbered.getByRole("switch", { name: "Weigh-ins" }).getAttribute("aria-checked")).toBe("true");
});

test("compact metadata toggle retains explicit settings save", async () => {
  const {save} = setup();
  const toggle = await screen.findByRole("switch", {name: "Automatic Metadata Refresh"});
  fireEvent.click(toggle);
  expect(toggle.getAttribute("aria-checked")).toBe("true");
  expect(save).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", {name: /save settings/i}));
  await waitFor(() => expect(save).toHaveBeenCalledWith(expect.objectContaining({periodic_sync_enabled: true})));
});


test("editing only exclusion terms enables revisioned settings save",async()=>{
 const {save}=setup();
 fireEvent.click(await screen.findByRole("button",{name:"Excluded Terms"}));
 const input=await screen.findByRole("textbox",{name:"Main Card exclusion terms"});
 await waitFor(()=>expect((input as HTMLTextAreaElement).value).toContain("pre show"));
 fireEvent.change(input,{target:{value:"special show\npress conference"}});
 const button=screen.getByRole("button",{name:/save settings/i});
 expect((button as HTMLButtonElement).disabled).toBe(false);
 fireEvent.click(button);
 await waitFor(()=>expect(save).toHaveBeenCalledWith(expect.objectContaining({revision:3,main_card_terms:["special show","press conference"]})));
});
