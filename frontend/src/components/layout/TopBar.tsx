// 顶部栏：品牌 + 项目切换器 + 环境色标占位 + 通知铃铛 + 用户菜单（含语言切换）。
// 信息架构依据 docs/prd/15-ui.md §1。
import { Bell, LogOut, User } from "lucide-react";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { LanguageSwitcher } from "./LanguageSwitcher";
import { ProjectSwitcher } from "./ProjectSwitcher";

/** 环境色标占位：后续接入 environment.color 后从 ctx 读取。 */
function EnvironmentBadge(): JSX.Element | null {
  // 占位：暂不展示具体色标，等环境上下文接入后补全（15-ui §1：prod🔴/stage🟠/test🔵/dev🟢）。
  return null;
}

export function TopBar(): JSX.Element {
  const { t } = useTranslation();
  return (
    <header className="flex h-14 items-center gap-3 border-b border-gray-200 bg-white px-4">
      <Link to="/" className="flex items-center gap-2 text-gray-900" aria-label={t("app.title")}>
        <span className="rounded bg-blue-600 px-2 py-0.5 text-sm font-bold text-white">DB</span>
        <span className="text-base font-semibold">DBHUB</span>
      </Link>
      <div className="mx-2 hidden h-6 w-px bg-gray-200 sm:block" aria-hidden />
      <ProjectSwitcher />
      <EnvironmentBadge />
      <div className="ml-auto flex items-center gap-1">
        <Button
          variant="ghost"
          size="icon"
          asChild
          aria-label={t("common.notifications")}
          title={t("common.notifications")}
        >
          <Link to="/notifications">
            <Bell className="h-5 w-5" aria-hidden />
          </Link>
        </Button>
        <LanguageSwitcher />
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              aria-label={t("common.userMenu")}
              aria-haspopup="menu"
            >
              <User className="h-5 w-5" aria-hidden />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" role="menu">
            <DropdownMenuLabel>{t("common.profile")}</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link to="/settings">
                <User className="h-4 w-4" aria-hidden />
                {t("common.profile")}
              </Link>
            </DropdownMenuItem>
            <DropdownMenuItem asChild>
              <Link to="/login">
                <LogOut className="h-4 w-4" aria-hidden />
                {t("common.logout")}
              </Link>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </header>
  );
}
