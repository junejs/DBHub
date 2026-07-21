// localStorage 安全访问：Node 22+ 实验性 localStorage 在未启用时为 undefined，
// 此处统一兜底，避免 zustand persist 中间件在非浏览器环境（含 vitest）抛错。
const memoryStore = new Map<string, string>();

function safeLocalStorage(): Storage | typeof memoryStore {
  try {
    if (typeof localStorage !== "undefined" && localStorage) {
      return localStorage;
    }
  } catch {
    // localStorage 访问可能抛错（某些隐私模式或 Node 实验性未启用）。
  }
  return memoryStore as unknown as Storage;
}

export const browserStorage = {
  getItem: (key: string): string | null => {
    const store = safeLocalStorage();
    if (store instanceof Map) {
      return (store.get(key) as string | undefined) ?? null;
    }
    return store.getItem(key);
  },
  setItem: (key: string, value: string): void => {
    const store = safeLocalStorage();
    if (store instanceof Map) {
      store.set(key, value);
    } else {
      store.setItem(key, value);
    }
  },
  removeItem: (key: string): void => {
    const store = safeLocalStorage();
    if (store instanceof Map) {
      store.delete(key);
    } else {
      store.removeItem(key);
    }
  },
} as unknown as Storage;

/** 测试/工具用：清空持久化数据。在浏览器走 localStorage.clear()，否则走内存。 */
export function clearPersistedStorage(): void {
  try {
    if (typeof localStorage !== "undefined" && localStorage) {
      localStorage.clear();
      return;
    }
  } catch {
    // ignore
  }
  memoryStore.clear();
}
