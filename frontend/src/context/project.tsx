// 项目上下文占位：当前所选项目。
// 后续接入 GET /v1/projects 后会演变为基于 hook 的真实数据流；
// 当前阶段仅提供壳，避免页面组件直接持服务端状态（D50/D51）。
import { createContext, type JSX, type ReactNode, useContext } from "react";

export interface CurrentProject {
  key: string;
  title: string;
}

interface ProjectContextValue {
  current: CurrentProject | null;
  projects: CurrentProject[];
  setCurrent: (project: CurrentProject) => void;
}

const ProjectContext = createContext<ProjectContextValue | null>(null);

export function ProjectProvider({
  children,
  value,
}: {
  children: ReactNode;
  value: ProjectContextValue;
}): JSX.Element {
  return <ProjectContext.Provider value={value}>{children}</ProjectContext.Provider>;
}

export function useProjectContext(): ProjectContextValue {
  const ctx = useContext(ProjectContext);
  if (!ctx) {
    throw new Error("useProjectContext must be used within a ProjectProvider");
  }
  return ctx;
}
