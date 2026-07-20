// 测试渲染工具：统一包装 QueryClientProvider + MemoryRouter + i18n + ProjectProvider。
// 组件/页面测试一律通过此 helper 渲染，避免重复样板。
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { RenderOptions, RenderResult } from "@testing-library/react";
import { render } from "@testing-library/react";
import type { JSX, ReactNode } from "react";
import { MemoryRouter } from "react-router-dom";
import { type CurrentProject, ProjectProvider } from "@/context/project";
import "@/i18n";

export interface AppRenderOptions extends Omit<RenderOptions, "wrapper"> {
  initialEntries?: string[];
  projects?: CurrentProject[];
  currentProject?: CurrentProject | null;
  queryClient?: QueryClient;
}

function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0, staleTime: 0 },
      mutations: { retry: false },
    },
  });
}

export function renderWithAppContext(
  ui: ReactNode,
  options: AppRenderOptions = {},
): RenderResult & { queryClient: QueryClient } {
  const {
    initialEntries = ["/"],
    projects = [],
    currentProject = null,
    queryClient = createQueryClient(),
  } = options;

  function Wrapper({ children }: { children: ReactNode }): JSX.Element {
    return (
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={initialEntries}>
          <ProjectProvider
            value={{
              current: currentProject,
              projects,
              setCurrent: () => {},
            }}
          >
            {children}
          </ProjectProvider>
        </MemoryRouter>
      </QueryClientProvider>
    );
  }

  const result = render(ui, { wrapper: Wrapper });
  return { ...result, queryClient };
}

export { createQueryClient };
