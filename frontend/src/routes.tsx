// 应用路由（react-router v7 数据路由之外的最简 RouterProvider 等价形态）。
// 占位页面在 src/pages；布局在 src/components/layout。
import type { JSX } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { NotFoundState } from "@/components/common/states";
import { RootLayout } from "@/components/layout/RootLayout";
import {
  AdminPlaceholderPage,
  AuditPage,
  ExportsPage,
  FavoritesPage,
  LoginPage,
  NotificationsPage,
  ResourcesPage,
  SettingsPage,
  WorkbenchPage,
} from "@/pages";

export function AppRoutes(): JSX.Element {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route element={<RootLayout />}>
        <Route index element={<Navigate to="/workbench" replace />} />
        <Route path="workbench" element={<WorkbenchPage />} />
        <Route path="resources" element={<ResourcesPage />} />
        <Route path="favorites" element={<FavoritesPage />} />
        <Route path="exports" element={<ExportsPage />} />
        <Route path="audit" element={<AuditPage />} />
        <Route path="notifications" element={<NotificationsPage />} />
        <Route path="admin/instances" element={<AdminPlaceholderPage />} />
        <Route path="admin/environments" element={<AdminPlaceholderPage />} />
        <Route path="admin/idps" element={<AdminPlaceholderPage />} />
        <Route path="admin/users" element={<AdminPlaceholderPage />} />
        <Route path="admin/settings" element={<AdminPlaceholderPage />} />
        <Route path="settings" element={<SettingsPage />} />
        <Route
          path="*"
          element={
            <div className="p-8">
              <NotFoundState />
            </div>
          }
        />
      </Route>
    </Routes>
  );
}
