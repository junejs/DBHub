// 时间/时区格式化基础（D23 i18n + 08-nfr §1.7）。
// 传输层为 RFC3339 UTC 字符串（契约），展示层按用户语言/时区格式化。

import type { Locale } from "date-fns";
import { format, formatDistanceToNow, parseISO } from "date-fns";
import { zhCN } from "date-fns/locale";

const ZH_LOCALE: Locale = zhCN;

export type AppLocale = "zh" | "en";

function localeFor(lang: AppLocale): Locale | undefined {
  return lang === "zh" ? ZH_LOCALE : undefined;
}

function toDate(value: string | Date): Date {
  return typeof value === "string" ? parseISO(value) : value;
}

/** 绝对时间展示（默认 yyyy-MM-dd HH:mm）。 */
export function formatDateTime(value: string | Date, lang: AppLocale = "zh"): string {
  const date = toDate(value);
  if (Number.isNaN(date.getTime())) return "";
  const pattern = lang === "zh" ? "yyyy-MM-dd HH:mm" : "MMM d, yyyy HH:mm";
  return format(date, pattern, { locale: localeFor(lang) });
}

/** 相对时间展示（如「3 分钟前」/「3 minutes ago」）。 */
export function formatRelative(value: string | Date, lang: AppLocale = "zh"): string {
  const date = toDate(value);
  if (Number.isNaN(date.getTime())) return "";
  return formatDistanceToNow(date, { addSuffix: true, locale: localeFor(lang) });
}
