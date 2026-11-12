import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { APIClient } from "@api/APIClient";
import { UFCProcessingSettings } from "@screens/UFCProcessingSettings";
import type { UFCProcessingSettings as ProcessingSettings } from "@app/types/UFCProcessing";
import { UFCAutomationControl, UFCProcessingAutomationControl } from "@screens/UFCAutomation";

afterEach(() => { cleanup(); vi.restoreAllMocks(); });

test("saves directly and labels the saved enabled and disabled states", async () => {
  vi.spyOn(APIClient.ufc, "automationSettings").mockResolvedValue({ revision: 4, enabled: false });
  const save = vi.spyOn(APIClient.ufc, "saveAutomationSettings").mockResolvedValueOnce({ revision: 5, enabled: true }).mockResolvedValueOnce({ revision: 6, enabled: false });
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(<QueryClientProvider client={client}><UFCAutomationControl /></QueryClientProvider>);
  fireEvent.click(await screen.findByRole("switch", {name:"Disabled"}));
  await waitFor(() => expect(save).toHaveBeenCalledWith({ revision: 4, enabled: true }));
  fireEvent.click(await screen.findByRole("switch", {name:"Enabled"}));
  await waitFor(() => expect(save).toHaveBeenLastCalledWith({ revision: 5, enabled: false }));
  await screen.findByRole("switch", {name:"Disabled"});
});

test("processing toggle preserves draft configuration and saves only its own flag",async()=>{
 let current:ProcessingSettings={revision:1,source_roots:["X:/downloads"],destination_root:"X:/library",event_folder_template:"event-template",main_file_template:"main",extra_file_template:"extra",component_folder:"Featurettes",conflict_policy:"review",extensions:[".mkv"],max_concurrent:1,max_attempts:3,sab_client_id:1,sab_category:"ufc",auto_process_completed:false,cleanup_sab_download:false};
 vi.spyOn(APIClient.ufc,"processingSettings").mockImplementation(async()=>current);
 vi.spyOn(APIClient.ufc,"downloadClients").mockResolvedValue([{id:1,name:"Fixture SAB"}]);
 vi.spyOn(APIClient.ufc,"librarySettings").mockResolvedValue({revision:1,running:false,run_id:"",last_scan:null,message:"",roots:[{id:"root",path:"X:/library",enabled:true,state:"available",message:""}]});
 vi.spyOn(APIClient.ufc,"sabCategories").mockResolvedValue(["ufc"]);
 const save=vi.spyOn(APIClient.ufc,"saveProcessingSettings").mockImplementation(async input=>{current={...input,revision:input.revision+1};return current;});
 const client=new QueryClient({defaultOptions:{queries:{retry:false}}});
 render(<QueryClientProvider client={client}><UFCProcessingSettings/><UFCProcessingAutomationControl field="auto_process_completed"/></QueryClientProvider>);
 const draft=await screen.findByDisplayValue("event-template");fireEvent.change(draft,{target:{value:"my draft"}});
 fireEvent.click(await screen.findByRole("switch",{name:"Disabled"}));await screen.findByRole("switch",{name:"Enabled"});
 expect(save).toHaveBeenCalledWith(expect.objectContaining({revision:1,event_folder_template:"event-template",auto_process_completed:true,cleanup_sab_download:false}));
 expect((draft as HTMLInputElement).value).toBe("my draft");
 fireEvent.click(screen.getByRole("button",{name:/save processing settings/i}));
 await waitFor(()=>expect(save).toHaveBeenLastCalledWith(expect.objectContaining({revision:2,event_folder_template:"my draft",auto_process_completed:true,cleanup_sab_download:false})));
});
