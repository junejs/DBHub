// 占位页面集合：本任务仅建立应用壳与路由，业务页面后续按 issue 落地。
import type { JSX } from "react";
import { useTranslation } from "react-i18next";
import { PageContainer, PlaceholderPage } from "@/components/layout/PageContainer";

export function WorkbenchPage(): JSX.Element {
  const { t } = useTranslation();
  return <PlaceholderPage title={t("nav.workbench")} description={t("placeholder.workbench")} />;
}

export function ResourcesPage(): JSX.Element {
  const { t } = useTranslation();
  return <PlaceholderPage title={t("nav.resources")} description={t("placeholder.resources")} />;
}

export function FavoritesPage(): JSX.Element {
  const { t } = useTranslation();
  return <PlaceholderPage title={t("nav.favorites")} description={t("placeholder.favorites")} />;
}

export function ExportsPage(): JSX.Element {
  const { t } = useTranslation();
  return <PlaceholderPage title={t("nav.exports")} description={t("placeholder.exports")} />;
}

export function AuditPage(): JSX.Element {
  const { t } = useTranslation();
  return <PlaceholderPage title={t("nav.audit")} description={t("placeholder.audit")} />;
}

export function NotificationsPage(): JSX.Element {
  const { t } = useTranslation();
  return (
    <PlaceholderPage
      title={t("common.notifications")}
      description={t("placeholder.notifications")}
    />
  );
}

export function AdminPlaceholderPage(): JSX.Element {
  const { t } = useTranslation();
  return <PlaceholderPage title={t("nav.admin")} description={t("placeholder.admin")} />;
}

export function SettingsPage(): JSX.Element {
  const { t } = useTranslation();
  return <PlaceholderPage title={t("common.profile")} description={t("placeholder.settings")} />;
}

/** 登录页占位：全屏路由（不走 RootLayout）。 */
export function LoginPage(): JSX.Element {
  const { t } = useTranslation();
  return (
    <div className="flex min-h-screen flex-col items-center justify-center bg-gray-50 p-8 text-center">
      <div className="mb-4 flex items-center gap-2">
        <span className="rounded bg-blue-600 px-2 py-0.5 text-sm font-bold text-white">DB</span>
        <span className="text-xl font-semibold text-gray-900">{t("app.title")}</span>
      </div>
      <PageContainer title={t("common.profile")} className="max-w-md">
        <div className="rounded-lg border border-dashed border-gray-300 bg-white p-10 text-sm text-gray-500">
          {t("placeholder.login")}
        </div>
      </PageContainer>
    </div>
  );
}
