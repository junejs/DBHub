// 客户端 UI 状态（Zustand）：语言偏好、侧边栏折叠态。
// 仅存「客户端 UI 状态」，服务端状态走 TanStack Query（CODING_STANDARDS §4.3）。
import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { type AppLanguage, DEFAULT_LANGUAGE, isAppLanguage } from "@/i18n";
import { browserStorage } from "@/lib/storage";

interface UiState {
  language: AppLanguage;
  sidebarCollapsed: boolean;
  setLanguage: (lang: AppLanguage) => void;
  toggleSidebar: () => void;
  setSidebarCollapsed: (collapsed: boolean) => void;
}

export const useUiStore = create<UiState>()(
  persist(
    (set) => ({
      language: readInitialLanguage(),
      sidebarCollapsed: false,
      setLanguage: (lang) => set({ language: lang }),
      toggleSidebar: () => set((s) => ({ sidebarCollapsed: !s.sidebarCollapsed })),
      setSidebarCollapsed: (collapsed) => set({ sidebarCollapsed: collapsed }),
    }),
    {
      name: "dbhub.ui",
      storage: createJSONStorage(() => browserStorage),
      partialize: (state) => ({
        language: state.language,
        sidebarCollapsed: state.sidebarCollapsed,
      }),
    },
  ),
);

function readInitialLanguage(): AppLanguage {
  try {
    const raw = browserStorage.getItem("dbhub.ui");
    if (!raw) return DEFAULT_LANGUAGE;
    const parsed = JSON.parse(raw) as { state?: { language?: unknown } };
    const lang = parsed?.state?.language;
    return isAppLanguage(lang) ? lang : DEFAULT_LANGUAGE;
  } catch {
    return DEFAULT_LANGUAGE;
  }
}

/** 读取当前语言偏好（用于 i18n 初始化前的同步读取，避免 SSR 不一致）。 */
export function readLanguage(): AppLanguage {
  return readInitialLanguage();
}

/** 持久化语言偏好（i18n languageChanged 事件回调中调用）。 */
export function persistLanguage(lang: string): void {
  if (!isAppLanguage(lang)) return;
  const current = useUiStore.getState().language;
  if (current !== lang) {
    useUiStore.getState().setLanguage(lang);
  }
}
