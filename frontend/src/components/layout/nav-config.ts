// 主导航配置：依据 15-ui.md §1 信息架构。
// 普通成员可见前 4 项（工作台/资源/收藏/导出）；审计/管理后台仅管理员可见，
// 当前阶段未接入权限，先以 placeholder 全量展示，由后续 IAM 落地时收敛。
import {
  ShieldCheck as AdminIcon,
  ClipboardList as AuditIcon,
  FolderTree as EnvironmentsIcon,
  FileOutput as ExportsIcon,
  Star as FavoritesIcon,
  KeyRound as IdpsIcon,
  Database as InstancesIcon,
  type LucideIcon,
  SlidersHorizontal as PlatformSettingsIcon,
  Box as ResourcesIcon,
  Settings as SettingsIcon,
  Users as UsersIcon,
  LayoutDashboard as WorkbenchIcon,
} from "lucide-react";
import type { TranslationKey } from "@/i18n/types";

export interface NavItem {
  to: string;
  labelKey: TranslationKey;
  icon: LucideIcon;
}

export interface NavGroup {
  id: string;
  items: NavItem[];
}

export const PRIMARY_NAV: NavGroup[] = [
  {
    id: "primary",
    items: [
      { to: "/workbench", labelKey: "nav.workbench", icon: WorkbenchIcon },
      { to: "/resources", labelKey: "nav.resources", icon: ResourcesIcon },
      { to: "/favorites", labelKey: "nav.favorites", icon: FavoritesIcon },
      { to: "/exports", labelKey: "nav.exports", icon: ExportsIcon },
    ],
  },
  {
    id: "admin",
    items: [
      { to: "/audit", labelKey: "nav.audit", icon: AuditIcon },
      { to: "/admin/instances", labelKey: "nav.adminInstances", icon: InstancesIcon },
      { to: "/admin/environments", labelKey: "nav.adminEnvironments", icon: EnvironmentsIcon },
      { to: "/admin/idps", labelKey: "nav.adminIdps", icon: IdpsIcon },
      { to: "/admin/users", labelKey: "nav.adminUsers", icon: UsersIcon },
      { to: "/admin/settings", labelKey: "nav.adminSettings", icon: PlatformSettingsIcon },
    ],
  },
];

export const SECONDARY_NAV: NavItem[] = [
  { to: "/notifications", labelKey: "common.notifications", icon: AuditIcon },
  { to: "/settings", labelKey: "common.profile", icon: SettingsIcon },
];

export const ADMIN_PATH_PREFIX = "/admin";

export type { TranslationKey } from "@/i18n/types";
export { AdminIcon };
