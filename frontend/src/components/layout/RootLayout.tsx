// 根布局：顶部栏 + 侧边导航 + 内容区（Outlet）。
// 所有受布局管理的路由都经此壳（登录页除外，需独立全屏路由）。
import type { JSX } from "react";
import { Outlet } from "react-router-dom";
import { SideNav } from "./SideNav";
import { TopBar } from "./TopBar";

export function RootLayout(): JSX.Element {
  return (
    <div className="flex min-h-screen flex-col bg-gray-50 text-gray-900">
      <TopBar />
      <div className="flex flex-1 overflow-hidden">
        <SideNav />
        <main id="main-content" className="flex-1 overflow-y-auto" tabIndex={-1}>
          <Outlet />
        </main>
      </div>
    </div>
  );
}
