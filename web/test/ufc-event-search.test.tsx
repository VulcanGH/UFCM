import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { APIClient } from "@api/APIClient";
import type { UFCEventValues } from "@app/types/UFC";
import type { UFCMatchingPreview } from "@app/types/UFCMatching";
import { UFCEventSearch } from "@screens/UFCEventSearch";
import type { ReactNode } from "react";

vi.mock("@tanstack/react-router", () => ({ Link: ({ to, children }: { to: string; children: ReactNode }) => <a href={to}>{children}</a> }));

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

test("hides rejected event/component/quality results until explicitly requested", async () => {
  vi.spyOn(APIClient.ufc, "libraryCoverage").mockResolvedValue({ configured: false, running: false, unavailable: false, review: 0, events: {} });
  vi.spyOn(APIClient.ufc, "downloadClients").mockResolvedValue([{ id: 9, name: "Saved SAB" }]);
  const submit = vi.spyOn(APIClient.ufc, "submitDownload").mockResolvedValue({ existing: false, job: { id: "owned", event_id: "local", component: "main_card", title: "UFC.300.1080p", source: "Configured feed", client_id: 9, client_name: "Saved SAB", category: "", state: "queued", remote_id: "sab-1", percent: 0, message: "Sent to SABnzbd", release_id: 1, revision: 2, created_at: "2026-11-10T16:00:00Z", updated_at: "2026-11-10T16:00:00Z" } });
  const values: UFCEventValues = { name: "UFC 300", type: "numbered", date: null, headliners: [], status: "completed" };
  vi.spyOn(APIClient.ufc, "event").mockResolvedValue({ id: "local", revision: 1, values, source: { ...values, page_id: 1, list_revision: 1, article_revision: 1, retrieved_at: "" }, corrections: {}, aliases: [], review: [], quality: { defaults: ["1080p"], override: null, effective: ["1080p"] } });
  const source = { id: 1, name: "Configured feed", type: "NEWZNAB" };
  vi.spyOn(APIClient.ufc, "searchSources").mockResolvedValue([source]);
  const match: UFCMatchingPreview = { version: "preview-3", title: "", normalized: "", status: "matched", identifiers: [], candidates: [], components: { status: "inferred", values: ["main_card"] }, quality: { status: "title assertions", values: ["1080p"] }, main_card: { status: "inferred label", reason: "", matched_terms: [], groups: [] }, notes: [] };
  vi.spyOn(APIClient.ufc, "searchEvent").mockResolvedValue({ event_revision:1,settings_revision:1,event_id: "local", component: "main_card", query: "UFC 300", resolutions: ["1080p"], sources: [{ source, count: 2, more: false }], results: [
    { candidate_id: "owned-token", source, title: "UFC.300.1080p", size: 1000, matched: true, assessment: "match", reasons: [], match, quality_check: { matched: true, allowed: ["1080p"], reason: "Accepted" } },
    { source, title: "UFC.300.Pre-Show.720p", size: 1000, matched: false, assessment: "needs review", reasons: ["Excluded for Main Card", "720p is not accepted"], match, quality_check: { matched: false, allowed: ["1080p"], reason: "Rejected" } },
    {candidate_id:"manual-token",published_at:"2026-11-01T12:00:00Z",source,title:"UFC.999.Prelims.1080p",size:1000,matched:false,assessment:"needs review",reasons:["Wrong event"],match,quality_check:{matched:true,allowed:["1080p"],reason:"Accepted"}}
  ] });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><UFCEventSearch id="local" onClose={() => {}} /></QueryClientProvider>);
  expect((await screen.findByRole("switch", { name: /Configured feed/ }) as HTMLElement).getAttribute("aria-checked")).toBe("true");
  fireEvent.click(screen.getByRole("button", { name: "Search releases" }));
  await screen.findByRole("heading", { name: "UFC.300.1080p" });
  expect(screen.queryByRole("heading", { name: "UFC.300.Pre-Show.720p" })).toBeNull();
  fireEvent.click(screen.getByRole("switch", { name: "Show Non-Matching Results And Reasons" }));
  expect(screen.getByRole("heading", { name: "UFC.300.Pre-Show.720p" })).toBeTruthy();
  expect(screen.getByText("720p is not accepted")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Download" }));
  expect(submit).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "Confirm download" }));
  await screen.findByRole("status");
  expect(submit).toHaveBeenCalledTimes(1);
  expect(submit).toHaveBeenCalledWith({ candidate_id: "owned-token", client_id: 9, category: "" });
  expect(screen.getByText(/Published.*2026/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button",{name:"Assign manually"}));
  expect((screen.getByRole("button",{name:"Confirm download"}) as HTMLButtonElement).disabled).toBe(true);
  fireEvent.change(screen.getByRole("textbox",{name:"Manual assignment reason"}),{target:{value:"Verified the release listing manually"}});
  fireEvent.click(screen.getByRole("button",{name:"Confirm download"}));
  await waitFor(()=>expect(submit).toHaveBeenCalledTimes(2));
  expect(submit).toHaveBeenLastCalledWith({candidate_id:"manual-token",client_id:9,category:"",manual_reason:"Verified the release listing manually",event_revision:1,settings_revision:1});
});
