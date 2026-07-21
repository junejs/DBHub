// MSW 服务端（node 端测试用）。
// 在 setup.ts 中通过 beforeAll/afterEach/afterAll 接入测试生命周期。
import { setupServer } from "msw/node";
import { handlers } from "./handlers";

export const server = setupServer(...handlers);
