import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { UFCActionButton } from "@screens/UFCActionButton";

afterEach(cleanup);
test("action colour returns to neutral after completion until a fresh hover",()=>{
  const click=vi.fn();
  const props={onClick:click,className:"text-gray-600 hover:text-blue-600 dark:text-gray-300 dark:hover:text-blue-500"};
  const {rerender}=render(<UFCActionButton {...props}>Refresh</UFCActionButton>);
  const button=screen.getByRole("button",{name:"Refresh"});
  fireEvent.pointerEnter(button);
  expect(button.className).toContain("hover:text-blue-600");
  fireEvent.click(button);
  expect(click).toHaveBeenCalledTimes(1);
  rerender(<UFCActionButton {...props} busy>Refreshing</UFCActionButton>);
  expect(button.className.split(" ")).toContain("text-blue-600");
  expect(button.getAttribute("aria-busy")).toBe("true");
  rerender(<UFCActionButton {...props}>Refresh</UFCActionButton>);
  expect(button.className.split(" ")).toContain("text-gray-600");
  expect(button.className).not.toContain("hover:text-blue");
  expect(button.className).toContain("focus-visible:outline");
  fireEvent.pointerLeave(button);
  fireEvent.pointerEnter(button);
  expect(button.className).toContain("hover:text-blue-600");
});
