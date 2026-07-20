import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { browserStorage } from "@/lib/storage";
import { renderWithAppContext } from "@/test/render";
import { LanguageSwitcher } from "./LanguageSwitcher";
import { SideNav } from "./SideNav";
import { TopBar } from "./TopBar";

describe("TopBar", () => {
  it("renders brand, project switcher, notification and user menu controls", () => {
    renderWithAppContext(<TopBar />, {
      projects: [{ key: "platform", title: "Platform" }],
      currentProject: { key: "platform", title: "Platform" },
    });
    expect(screen.getByRole("link", { name: "DBHUB" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "选择项目" })).toBeInTheDocument();
    // 通知入口通过 Button asChild + Link 渲染为 anchor。
    expect(screen.getByRole("link", { name: "通知" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "用户菜单" })).toBeInTheDocument();
  });
});

describe("SideNav", () => {
  it("renders every primary and secondary nav link", () => {
    renderWithAppContext(<SideNav />);
    const nav = screen.getByRole("navigation");
    for (const label of ["工作台", "资源", "收藏", "导出中心", "审计"]) {
      expect(within(nav).getByRole("link", { name: label })).toBeInTheDocument();
    }
    expect(within(nav).getByRole("link", { name: "实例管理" })).toBeInTheDocument();
    expect(within(nav).getByRole("link", { name: "个人设置" })).toBeInTheDocument();
  });
});

describe("LanguageSwitcher", () => {
  it("switches between zh and en and persists the choice", async () => {
    renderWithAppContext(<LanguageSwitcher />);
    const trigger = screen.getByRole("button", { name: "语言" });
    await userEvent.click(trigger);
    expect(screen.getByRole("menuitemradio", { name: "简体中文" })).toHaveAttribute(
      "aria-checked",
      "true",
    );

    await userEvent.click(screen.getByRole("menuitemradio", { name: "English" }));
    // store 持久化
    expect(JSON.parse(browserStorage.getItem("dbhub.ui") ?? "{}").state.language).toBe("en");
  });
});
