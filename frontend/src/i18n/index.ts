// i18n 基线（D23 中英双语，react-i18next）。
// 用户可见文案一律走 t()/i18n key，禁止硬编码（CODING_STANDARDS §5）。
import i18n from "i18next";
import { initReactI18next } from "react-i18next";
import { persistLanguage, readLanguage } from "@/stores/ui";

import en from "./locales/en.json";
import zh from "./locales/zh.json";

export const SUPPORTED_LANGUAGES = ["zh", "en"] as const;
export type AppLanguage = (typeof SUPPORTED_LANGUAGES)[number];

export const DEFAULT_LANGUAGE: AppLanguage = "zh";

export function isAppLanguage(value: unknown): value is AppLanguage {
  return value === "zh" || value === "en";
}

const initialLanguage = readLanguage();

void i18n.use(initReactI18next).init({
  resources: {
    zh: { translation: zh },
    en: { translation: en },
  },
  lng: initialLanguage,
  fallbackLng: DEFAULT_LANGUAGE,
  interpolation: {
    escapeValue: false,
  },
  returnNull: false,
});

i18n.on("languageChanged", (lng) => {
  persistLanguage(lng);
  if (typeof document !== "undefined") {
    document.documentElement.lang = lng;
  }
});

export default i18n;
