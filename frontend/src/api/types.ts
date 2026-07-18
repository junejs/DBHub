// 手写 API 类型，对应 openapi.yaml 的 components.schemas（唯一事实来源）。
// 说明：契约更新后需手动同步本文件（TS7 下 openapi-typescript 尚不支持，见决策记录）。
// JSON 字段沿用契约的 snake_case。

/** 统一错误响应（openapi.yaml#/components/schemas/Error）。 */
export interface ApiError {
  code: number;
  message: string;
  details?: ErrorDetail[];
}

export interface ErrorDetail {
  "@type"?: string;
  reason?: string;
  domain?: string;
  metadata?: Record<string, string>;
  field_violations?: FieldViolation[];
}

export interface FieldViolation {
  field: string;
  description: string;
}

/** 通用分页入参（openapi.yaml page_size / page_token）。 */
export interface PageParams {
  page_size?: number;
  page_token?: string;
}

// ───────────────────────── Project ─────────────────────────

export interface Project {
  name?: string; // projects/{key}
  key?: string;
  title?: string;
  description?: string;
  settings?: Record<string, unknown>;
  create_time?: string;
  update_time?: string;
}

export interface ListProjectsResponse {
  items: Project[];
  next_page_token?: string;
}
