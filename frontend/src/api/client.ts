// 手写的薄类型化 HTTP 客户端：fetch + JSON + Cookie 会话 + CSRF 双提交。
// 所有路径相对 /v1（dev 经 Vite 代理转给后端，prod 经 nginx）。

import type { ApiError } from "./types";

const BASE_URL = "/v1";

/** 带状态码与解析后错误体的异常，供 TanStack Query 的 error 读取。 */
export class ApiRequestError extends Error {
  constructor(
    public readonly status: number,
    public readonly body: ApiError | undefined,
    message?: string,
  ) {
    super(message ?? body?.message ?? `HTTP ${status}`);
    this.name = "ApiRequestError";
  }

  /** 提取稳定业务错误码（details[].reason），便于 UI 分支处理。 */
  get reason(): string | undefined {
    return this.body?.details?.find((d) => d.reason)?.reason;
  }
}

// CSRF：写操作（POST/PATCH/DELETE）须带 X-Csrf-Token（首屏 GET /v1/auth/csrf 下发）。
let csrfToken: string | null = null;

export function setCsrfToken(token: string): void {
  csrfToken = token;
}

type QueryValue = string | number | boolean | undefined;
type Query = Record<string, QueryValue>;

interface RequestOptions {
  signal?: AbortSignal;
  body?: unknown;
  query?: Query;
}

function buildUrl(path: string, query?: Query): string {
  if (!query) return `${BASE_URL}${path}`;
  const qs = Object.entries(query)
    .filter(([, v]) => v !== undefined && v !== "")
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`)
    .join("&");
  return qs ? `${BASE_URL}${path}?${qs}` : `${BASE_URL}${path}`;
}

function headers(method: string): Record<string, string> {
  const h: Record<string, string> = {
    Accept: "application/json",
  };
  if (method !== "GET" && method !== "DELETE") {
    h["Content-Type"] = "application/json";
  }
  if (csrfToken && method !== "GET" && method !== "HEAD") {
    h["X-Csrf-Token"] = csrfToken;
  }
  return h;
}

async function request<T>(method: string, path: string, opts: RequestOptions = {}): Promise<T> {
  const res = await fetch(buildUrl(path, opts.query), {
    method,
    credentials: "include", // 走 HTTP-only Cookie 会话
    signal: opts.signal,
    headers: headers(method),
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
  });

  if (res.status === 204) {
    return undefined as T;
  }

  const text = await res.text();
  const data = text ? (JSON.parse(text) as unknown) : undefined;
  if (!res.ok) {
    throw new ApiRequestError(res.status, data as ApiError | undefined);
  }
  return data as T;
}

export const api = {
  get: <T>(path: string, opts?: RequestOptions) => request<T>("GET", path, opts),
  post: <T>(path: string, body?: unknown, opts?: RequestOptions) =>
    request<T>("POST", path, { ...opts, body }),
  patch: <T>(path: string, body?: unknown, opts?: RequestOptions) =>
    request<T>("PATCH", path, { ...opts, body }),
  delete: <T>(path: string, opts?: RequestOptions) => request<T>("DELETE", path, opts),
};
