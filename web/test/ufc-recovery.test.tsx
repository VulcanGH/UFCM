import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { APIClient } from "@api/APIClient";
import { UFCCatalogueRecovery } from "@screens/UFCCatalogueRecovery";

afterEach(()=>{cleanup();vi.restoreAllMocks();});

test("catalogue reset requires its own acknowledgement and confirmation",async()=>{
  vi.spyOn(APIClient.ufc,"catalogueStatus").mockResolvedValue({running:false,interrupted:false,last_success:null,last_error:"",report:{diagnostics:[],diagnostics_omitted:0}});
  const reset=vi.spyOn(APIClient.ufc,"resetCatalogue").mockResolvedValue({reset:true});
  const client=new QueryClient({defaultOptions:{queries:{retry:false}}});
  render(<QueryClientProvider client={client}><UFCCatalogueRecovery /></QueryClientProvider>);
  const button=screen.getByRole("button",{name:"Reset event catalogue"});
  await waitFor(()=>expect((button as HTMLButtonElement).disabled).toBe(false));
  fireEvent.click(button);
  const confirm=screen.getByRole("button",{name:"Confirm reset catalogue"});
  expect((confirm as HTMLButtonElement).disabled).toBe(true);
  expect(reset).not.toHaveBeenCalled();
  expect(screen.getByText(/all disk files are kept/)).toBeTruthy();
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(confirm);
  await waitFor(()=>expect(reset).toHaveBeenCalledTimes(1));
});
test("historical rebuild requires warning acknowledgement and confirmation",async()=>{
  vi.spyOn(APIClient.ufc,"catalogueStatus").mockResolvedValue({running:false,interrupted:false,last_success:null,last_error:"",report:{diagnostics:[],diagnostics_omitted:0}});
  const rebuild=vi.spyOn(APIClient.ufc,"refreshCatalogue").mockResolvedValue({coalesced:false});
  const client=new QueryClient({defaultOptions:{queries:{retry:false}}});
  render(<QueryClientProvider client={client}><UFCCatalogueRecovery /></QueryClientProvider>);
  const button=screen.getByRole("button",{name:"Rebuild historical catalogue"});
  await waitFor(()=>expect((button as HTMLButtonElement).disabled).toBe(false));
  fireEvent.click(button);
  const confirm=screen.getByRole("button",{name:"Confirm rebuild"});
  expect((confirm as HTMLButtonElement).disabled).toBe(true);
  expect(rebuild).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("checkbox"));
  fireEvent.click(confirm);
  await waitFor(()=>expect(rebuild).toHaveBeenCalledWith("rebuild",true));
});
