import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import {
  ConcurrentModificationState,
  EmptyState,
  ErrorState,
  ForbiddenState,
  LoadingState,
  NotFoundState,
  RateLimitedState,
} from "./states";

describe("common states", () => {
  it("LoadingState renders spinner with default label", () => {
    render(<LoadingState />);
    expect(screen.getByRole("status", { name: "加载中…" })).toBeInTheDocument();
  });

  it("LoadingState renders skeleton variant", () => {
    render(<LoadingState variant="skeleton" />);
    expect(screen.getByRole("status", { name: "加载中…" })).toBeInTheDocument();
  });

  it("EmptyState renders default empty label and supports overrides", () => {
    const { rerender } = render(<EmptyState />);
    expect(screen.getByText("暂无数据")).toBeInTheDocument();
    rerender(<EmptyState title="无可见库" description="联系 projectOwner" />);
    expect(screen.getByText("无可见库")).toBeInTheDocument();
    expect(screen.getByText("联系 projectOwner")).toBeInTheDocument();
  });

  it("ErrorState shows reason and retry button", async () => {
    const onRetry = vi.fn();
    render(<ErrorState reason="QUERY_ROW_LIMIT_EXCEEDED" onRetry={onRetry} />);
    expect(screen.getByText("QUERY_ROW_LIMIT_EXCEEDED")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "重试" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
  });

  it("ForbiddenState renders 403 copy", () => {
    render(<ForbiddenState />);
    expect(screen.getByText("无权访问")).toBeInTheDocument();
  });

  it("NotFoundState renders 404 copy", () => {
    render(<NotFoundState />);
    expect(screen.getByText("页面不存在")).toBeInTheDocument();
  });

  it("RateLimitedState renders rate-limit copy", () => {
    render(<RateLimitedState />);
    expect(screen.getByText("操作过于频繁")).toBeInTheDocument();
  });

  it("ConcurrentModificationState triggers refresh handler", async () => {
    const onRefresh = vi.fn();
    render(<ConcurrentModificationState onRefresh={onRefresh} />);
    await userEvent.click(screen.getByRole("button", { name: "刷新" }));
    expect(onRefresh).toHaveBeenCalledTimes(1);
  });
});
