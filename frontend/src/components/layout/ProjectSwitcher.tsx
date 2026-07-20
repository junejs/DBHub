// 项目切换器占位：当前未接入 listProjects 数据流，
// 仅作为顶部布局占位渲染；后续接 hook 时替换内部实现。
import { ChevronDown, Folder } from "lucide-react";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { type CurrentProject, useProjectContext } from "@/context/project";

export function ProjectSwitcher(): JSX.Element {
  const { t } = useTranslation();
  const { current, projects, setCurrent } = useProjectContext();

  function select(p: CurrentProject): void {
    setCurrent(p);
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          aria-haspopup="menu"
          aria-label={t("common.projectSelect")}
        >
          <Folder className="h-4 w-4" aria-hidden />
          <span className="max-w-[14ch] truncate font-medium">
            {current ? current.title : t("common.projectNone")}
          </span>
          <ChevronDown className="h-4 w-4 opacity-60" aria-hidden />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" role="menu">
        <DropdownMenuLabel>{t("common.project")}</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {projects.length === 0 ? (
          <DropdownMenuItem disabled>{t("common.projectNone")}</DropdownMenuItem>
        ) : (
          projects.map((p) => (
            <DropdownMenuItem
              key={p.key}
              role="menuitemradio"
              aria-checked={p.key === current?.key}
              onSelect={() => select(p)}
            >
              {p.title}
            </DropdownMenuItem>
          ))
        )}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
