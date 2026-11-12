import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { ReactNode } from "react";
import { APIClient } from "@api/APIClient";
import { UFCDownloads } from "@screens/UFCDownloads";
import type { UFCDownload } from "@app/types/UFCDownload";

vi.mock("@tanstack/react-router", () => ({Link:({to,children}:{to:string;children:ReactNode})=><a href={to}>{children}</a>}));
afterEach(()=>{cleanup();vi.restoreAllMocks();});

test.each([["Remove", "Confirm Remove"], ["Clear All", "Confirm Clear All"]])("dismisses download cards using %s only after confirmation",async(button, confirm)=>{
  const job: UFCDownload = {id:"owned",revision:1,event_id:"event",component:"main_card",title:"UFC 303 fixture",source:"Saved indexer",client_id:1,client_name:"Saved SAB",category:"",state:"downloading",remote_id:"fake",percent:30,message:"",release_id:1,created_at:"2026-11-10T12:00:00Z",updated_at:"2026-11-10T12:00:00Z"};
  let removed=false;
  const second = {...job, id:"second", title:"Second download"};
  vi.spyOn(APIClient.ufc,"downloads").mockImplementation(async()=>removed?[]:button === "Clear All" ? [job, second] : [job]);
  vi.spyOn(APIClient.ufc,"processingJobs").mockResolvedValue([]);
  const remove=vi.spyOn(APIClient.ufc,"removeDownload").mockImplementation(async()=>{removed=true;});
  const client=new QueryClient({defaultOptions:{queries:{retry:false}}});
  render(<QueryClientProvider client={client}><UFCDownloads /></QueryClientProvider>);
  await screen.findByRole("heading",{name:job.title});
  fireEvent.click(screen.getByRole("button",{name:button}));
  expect(remove).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button",{name:confirm}));
  await waitFor(()=>expect(remove).toHaveBeenCalledWith("owned"));
  if (button === "Clear All") await waitFor(()=>expect(remove).toHaveBeenCalledWith("second"));
  await waitFor(()=>expect(screen.queryByRole("heading",{name:job.title})).toBeNull());
});

 test("verified copies use saved details and dismissed copy jobs stay hidden", async () => {
 const job: UFCDownload = {id:"owned",revision:1,event_id:"event",component:"main_card",title:"Verified download",source:"Saved indexer",client_id:1,client_name:"Saved SAB",category:"",state:"completed",remote_id:"fake",percent:100,message:"",release_id:1,created_at:"2026-11-10T12:00:00Z",updated_at:"2026-11-10T12:00:00Z"};
 vi.spyOn(APIClient.ufc,"downloads").mockResolvedValue([job]);
 const copy = {id:"copy", acquisition_id:job.id,event_id:"event",component:"main_card",destination_root:"X:/fixture",state:"completed" as const,resolution:"" as const,message:"",created_at:"",updated_at:"",cleanup_requested:true,cleanup_state:"done",cleanup_message:"",files:[]};
 vi.spyOn(APIClient.ufc,"processingJobs").mockResolvedValue([copy, {...copy,id:"dismissed",acquisition_id:"dismissed",message:"Hidden copy"}]);
 const preview=vi.spyOn(APIClient.ufc,"previewProcessing");
 const client=new QueryClient({defaultOptions:{queries:{retry:false}}});
 render(<QueryClientProvider client={client}><UFCDownloads /></QueryClientProvider>);
 await screen.findByText("Source Deleted");
 expect(screen.queryByRole("button",{name:"Preview"})).toBeNull();
 expect(screen.queryByText("Hidden copy")).toBeNull();
 expect(preview).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole("button",{name:"Details"}));
 expect(screen.getByRole("region",{name:/library copy job/i})).toBeTruthy();
 });
