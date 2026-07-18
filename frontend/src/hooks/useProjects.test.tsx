import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useProjects } from "./useProjects";

function createWrapper(client: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  };
}

afterEach(() => vi.unstubAllGlobals());

describe("useProjects", () => {
  it("returns items on success", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({
              items: [{ key: "platform", title: "Platform" }],
            }),
            { status: 200 },
          ),
      ),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    const { result } = renderHook(() => useProjects(), {
      wrapper: createWrapper(client),
    });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(result.current.data?.items[0].key).toBe("platform");
  });

  it("propagates error as ApiRequestError shape on non-2xx", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ code: 401, message: "unauthorized" }), {
            status: 401,
          }),
      ),
    );
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });

    const { result } = renderHook(() => useProjects(), {
      wrapper: createWrapper(client),
    });

    await waitFor(() => expect(result.current.isError).toBe(true));
    expect(result.current.error?.message).toBe("unauthorized");
  });
});
