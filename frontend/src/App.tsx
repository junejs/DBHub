import { type JSX, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { ProjectProvider } from "@/context/project";
import { useUiStore } from "@/stores/ui";
import { AppRoutes } from "./routes";

/** 应用根：装配 i18n、ProjectProvider、路由。QueryClient 由 main.tsx 注入。 */
export default function App(): JSX.Element {
  const { i18n } = useTranslation();
  const language = useUiStore((s) => s.language);

  // 启动时根据持久化语言同步 i18next（防止 i18n 模块加载早于 store 初始化）。
  useEffect(() => {
    if (i18n.language !== language) {
      void i18n.changeLanguage(language);
    }
  }, [i18n, language]);

  return (
    <ProjectProvider
      value={{
        current: null,
        projects: [],
        setCurrent: () => {},
      }}
    >
      <AppRoutes />
    </ProjectProvider>
  );
}
