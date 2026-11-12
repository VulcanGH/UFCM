import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { UFCStatusPanel } from "@screens/UFC";

afterEach(cleanup);

test("shows disabled capabilities without acquisition controls", () => {
  render(<UFCStatusPanel pending={false} status={{product_slug: "ufc-manager", product_name: "UFC Manager", schema_ready: true, settings_revision: 1, automatic_acquisition_enabled: false, processing_enabled: false}} />);
  expect(screen.getByRole("group", {name: "Automation Settings"})).toBeTruthy();
  expect(screen.getAllByText("Disabled")).toHaveLength(1);
  expect(screen.getAllByText("Unavailable")).toHaveLength(2);
  expect(screen.queryByRole("button")).toBeNull();
});

test("shows loading and useful errors", () => {
  const { rerender } = render(<UFCStatusPanel pending />);
  expect(screen.getByRole("status").textContent).toContain("Loading");
  rerender(<UFCStatusPanel pending={false} error={new Error("Please retry.")} />);
  expect(screen.getByRole("alert").textContent).toContain("Please retry");
});
