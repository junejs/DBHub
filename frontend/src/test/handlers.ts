// MSW 默认 handlers：占位返回，覆盖前端可能发出的请求。
// 各测试可 override（参见 MSW server.use()）。
import { HttpResponse, http } from "msw";

export const handlers = [
  http.get("/v1/projects", () =>
    HttpResponse.json({
      items: [{ key: "platform", title: "Platform" }],
      next_page_token: null,
    }),
  ),
];
