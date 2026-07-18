import { api } from './client'
import type { ListProjectsResponse, PageParams } from './types'

/** GET /v1/projects — 列出当前用户可见的项目。 */
export function listProjects(
  params: PageParams = {},
  signal?: AbortSignal,
): Promise<ListProjectsResponse> {
  return api.get<ListProjectsResponse>('/projects', {
    query: { page_size: params.page_size, page_token: params.page_token },
    signal,
  })
}
