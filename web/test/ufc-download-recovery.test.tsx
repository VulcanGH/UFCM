import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { APIClient } from "@api/APIClient";
import type { UFCDownload } from "@app/types/UFCDownload";
import { UFCDownloadRecovery } from "@screens/UFCDownloadRecovery";

afterEach(()=>{cleanup();vi.restoreAllMocks();});

test("recovery requires confirmation and keeps inputs after a revision conflict",async()=>{
 const job:UFCDownload={id:"intent",event_id:"event",component:"main_card",title:"UFC.300.1080p",source:"mock",client_id:1,client_name:"SAB",category:"",state:"uncertain",remote_id:"",percent:0,message:"",release_id:1,revision:2,created_at:"",updated_at:""};
 const recover=vi.spyOn(APIClient.ufc,"recoverDownload").mockRejectedValueOnce(new Error("Download revision changed"));
 const queryClient=new QueryClient({defaultOptions:{mutations:{retry:false}}});
 render(<QueryClientProvider client={queryClient}><UFCDownloadRecovery job={job}/></QueryClientProvider>);
 fireEvent.click(screen.getByRole("button",{name:"Recover existing SAB job"}));
 fireEvent.change(screen.getByRole("textbox",{name:"SAB job ID"}),{target:{value:"sab-known"}});
 expect((screen.getByRole("button",{name:"Verify and bind"}) as HTMLButtonElement).disabled).toBe(true);
 fireEvent.click(screen.getByRole("checkbox"));
 fireEvent.click(screen.getByRole("button",{name:"Verify and bind"}));
 await screen.findByRole("alert");
 expect((screen.getByRole("textbox",{name:"SAB job ID"}) as HTMLInputElement).value).toBe("sab-known");
 expect(recover).toHaveBeenCalledWith("intent",{revision:2,remote_id:"sab-known",confirm:true});
 recover.mockResolvedValueOnce({...job,revision:3,remote_id:"sab-known",state:"queued"});
 fireEvent.click(screen.getByRole("button",{name:"Verify and bind"}));
 await waitFor(()=>expect(screen.queryByRole("textbox",{name:"SAB job ID"})).toBeNull());
 expect(recover).toHaveBeenCalledTimes(2);
});
