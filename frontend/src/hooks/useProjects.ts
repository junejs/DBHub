import { useQuery } from '@tanstack/react-query'
import { listProjects } from '@/api/projects'

/** query key 工厂，便于按需失效/预取。 */
export const projectsKeys = {
  all: ['projects'] as const,
  list: (params: { pageSize?: number; pageToken?: string }) =>
    ['projects', 'list', params] as const,
}

/**
 * useProjects — 列出项目。
 * 入参用驼峰（前端惯例），内部映射为契约的 snake_case 查询参数。
 */
export function useProjects(params: { pageSize?: number; pageToken?: string } = {}) {
  return useQuery({
    queryKey: projectsKeys.list(params),
    queryFn: ({ signal }) =>
      listProjects(
        { page_size: params.pageSize, page_token: params.pageToken },
        signal,
      ),
  })
}
