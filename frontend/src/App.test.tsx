import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import App from "./App";

describe("App", () => {
  it("renders title", () => {
    render(<App />);
    expect(screen.getByText("DBHUB")).toBeInTheDocument();
  });

  it("increments count on click", async () => {
    render(<App />);
    const button = screen.getByRole("button", { name: /count is 0/i });
    await userEvent.click(button);
    expect(screen.getByRole("button", { name: /count is 1/i })).toBeInTheDocument();
  });
});
