// 侧边导航：按 15-ui.md §1 主导航分组。
// 高亮当前激活路由（NavLink aria-current），后续接入 IAM 后按角色显隐。
import type { JSX } from "react";
import { useTranslation } from "react-i18next";
import { NavLink } from "react-router-dom";
import { cn } from "@/lib/utils";
import { PRIMARY_NAV, SECONDARY_NAV } from "./nav-config";

export function SideNav(): JSX.Element {
  const { t } = useTranslation();
  return (
    <nav
      aria-label={t("app.title")}
      className="flex h-full w-56 shrink-0 flex-col border-r border-gray-200 bg-white"
    >
      <div className="flex-1 overflow-y-auto p-3">
        {PRIMARY_NAV.map((group, idx) => (
          <div key={group.id} className={cn(idx > 0 && "mt-6")}>
            <ul className="flex flex-col gap-0.5">
              {group.items.map((item) => {
                const Icon = item.icon;
                return (
                  <li key={item.to}>
                    <NavLink
                      to={item.to}
                      className={({ isActive }) =>
                        cn(
                          "flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                          isActive
                            ? "bg-blue-50 text-blue-700"
                            : "text-gray-700 hover:bg-gray-100 hover:text-gray-900",
                        )
                      }
                    >
                      <Icon className="h-4 w-4" aria-hidden />
                      <span>{t(item.labelKey)}</span>
                    </NavLink>
                  </li>
                );
              })}
            </ul>
          </div>
        ))}
      </div>
      <div className="border-t border-gray-200 p-3">
        <ul className="flex flex-col gap-0.5">
          {SECONDARY_NAV.map((item) => {
            const Icon = item.icon;
            return (
              <li key={item.to}>
                <NavLink
                  to={item.to}
                  className={({ isActive }) =>
                    cn(
                      "flex items-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                      isActive
                        ? "bg-blue-50 text-blue-700"
                        : "text-gray-700 hover:bg-gray-100 hover:text-gray-900",
                    )
                  }
                >
                  <Icon className="h-4 w-4" aria-hidden />
                  <span>{t(item.labelKey)}</span>
                </NavLink>
              </li>
            );
          })}
        </ul>
      </div>
    </nav>
  );
}
