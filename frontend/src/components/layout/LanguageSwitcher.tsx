// 语言切换器：中/英 即时切换（D23）。
// 通过 useUiStore 持久化到 localStorage，并通过 i18n.changeLanguage 同步触发 react-i18next 重渲染。
import { Check, Globe } from "lucide-react";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { type AppLanguage, SUPPORTED_LANGUAGES } from "@/i18n";
import { useUiStore } from "@/stores/ui";

const LABELS: Record<AppLanguage, string> = {
  zh: "简体中文",
  en: "English",
};

export function LanguageSwitcher(): JSX.Element {
  const { t, i18n } = useTranslation();
  const current = useUiStore((s) => s.language);

  function select(lang: AppLanguage): void {
    void i18n.changeLanguage(lang);
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" aria-label={t("common.language")} aria-haspopup="menu">
          <Globe className="h-5 w-5" aria-hidden />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" role="menu">
        {SUPPORTED_LANGUAGES.map((lang) => (
          <DropdownMenuItem
            key={lang}
            role="menuitemradio"
            aria-checked={lang === current}
            onSelect={() => select(lang)}
            className="justify-between"
          >
            <span>{LABELS[lang]}</span>
            {lang === current ? <Check className="h-4 w-4" aria-hidden /> : null}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
