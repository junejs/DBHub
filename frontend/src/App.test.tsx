import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import App from "./App";
import { renderWithAppContext } from "./test/render";

describe("App", () => {
  it("renders primary nav and workbench placeholder on /workbench", async () => {
    renderWithAppContext(<App />, { initialEntries: ["/workbench"] });
    const nav = await screen.findByRole("navigation");
    expect(within(nav).getByRole("link", { name: "工作台" })).toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "资源" })).toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "收藏" })).toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "导出中心" })).toBeInTheDocument();
    expect(await screen.findByText("SQL 查询工作台占位")).toBeInTheDocument();
  });

  it("redirects / to /workbench", async () => {
    renderWithAppContext(<App />, { initialEntries: ["/"] });
    expect(await screen.findByText("SQL 查询工作台占位")).toBeInTheDocument();
  });

  it("renders 404 state for unknown routes", async () => {
    renderWithAppContext(<App />, { initialEntries: ["/this-route-does-not-exist"] });
    expect(await screen.findByText("页面不存在")).toBeInTheDocument();
  });

  it("switches language instantly via the language switcher", async () => {
    renderWithAppContext(<App />, { initialEntries: ["/workbench"] });
    const switcher = await screen.findByLabelText("语言");
    await userEvent.click(switcher);
    const enItem = await screen.findByRole("menuitemradio", { name: /English/ });
    await userEvent.click(enItem);

    expect(await screen.findByRole("link", { name: "Workbench" })).toBeInTheDocument();
  });
});
