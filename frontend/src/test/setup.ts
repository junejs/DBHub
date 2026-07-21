// Vitest 测试环境初始化：jest-dom + MSW server 生命周期 + i18n/store 重置。
import "@testing-library/jest-dom";
import { afterAll, afterEach, beforeAll, beforeEach } from "vitest";
import i18n, { DEFAULT_LANGUAGE } from "@/i18n";
import { clearPersistedStorage } from "@/lib/storage";
import { useUiStore } from "@/stores/ui";
import { server } from "./server";

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));

beforeEach(() => {
  useUiStore.setState({ language: DEFAULT_LANGUAGE, sidebarCollapsed: false });
});

afterEach(() => {
  server.resetHandlers();
  clearPersistedStorage();
  void i18n.changeLanguage(DEFAULT_LANGUAGE);
});

afterAll(() => server.close());
