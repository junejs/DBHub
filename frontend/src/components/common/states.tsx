// 全站通用状态组件（15-ui.md §4 交互状态约定）。
// - 加载：骨架屏 / spinner
// - 空：空状态插画位 + 引导
// - 错误：顶部错误条 + 结构化信息（错误码 + 建议 action）
// - 403 无权限 / 404 路由不存在
// - 限流 RATE_LIMITED
// - 乐观锁冲突 CONCURRENT_MODIFICATION
import { AlertTriangle, Ban, Clock, FileQuestion, Lock, RefreshCw } from "lucide-react";
import type { JSX, ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface StateShellProps {
  icon: ReactNode;
  title: string;
  description?: string;
  action?: ReactNode;
  className?: string;
  children?: ReactNode;
}

function StateShell({
  icon,
  title,
  description,
  action,
  className,
  children,
}: StateShellProps): JSX.Element {
  return (
    <div
      className={cn(
        "flex min-h-[40vh] flex-col items-center justify-center gap-3 p-8 text-center",
        className,
      )}
      role="status"
      aria-live="polite"
    >
      <div className="text-gray-400" aria-hidden>
        {icon}
      </div>
      <div>
        <p className="text-base font-semibold text-gray-900">{title}</p>
        {description ? <p className="mt-1 text-sm text-gray-500">{description}</p> : null}
      </div>
      {children}
      {action}
    </div>
  );
}

/** 加载占位（默认 spinner；可用 variant="skeleton" 切换）。 */
export function LoadingState({
  label,
  variant = "spinner",
  className,
}: {
  label?: string;
  variant?: "spinner" | "skeleton";
  className?: string;
}): JSX.Element {
  const { t } = useTranslation();
  const text = label ?? t("common.loading");
  if (variant === "skeleton") {
    return (
      <div
        className={cn("flex flex-col gap-3 p-4", className)}
        role="status"
        aria-label={text}
        aria-live="polite"
      >
        <div className="h-4 w-1/3 animate-pulse rounded bg-gray-200" />
        <div className="h-4 w-2/3 animate-pulse rounded bg-gray-200" />
        <div className="h-4 w-1/2 animate-pulse rounded bg-gray-200" />
      </div>
    );
  }
  return (
    <div
      className={cn("flex items-center justify-center gap-2 p-8 text-gray-500", className)}
      role="status"
      aria-label={text}
      aria-live="polite"
    >
      <RefreshCw className="h-4 w-4 animate-spin" aria-hidden />
      <span className="text-sm">{text}</span>
    </div>
  );
}

/** 空状态：illustration 槽位 + 引导文案。 */
export function EmptyState({
  title,
  description,
  action,
  className,
}: {
  title?: string;
  description?: string;
  action?: ReactNode;
  className?: string;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <StateShell
      icon={<FileQuestion className="h-10 w-10" />}
      title={title ?? t("common.empty")}
      description={description}
      action={action}
      className={className}
    />
  );
}

interface ErrorStateProps {
  /** 稳定业务错误码（ApiRequestError.reason）。 */
  reason?: string;
  message?: string;
  description?: string;
  onRetry?: () => void;
  className?: string;
}

/** 通用错误状态。reason 用于让用户对照文档/联系管理员。 */
export function ErrorState({
  reason,
  message,
  description,
  onRetry,
  className,
}: ErrorStateProps): JSX.Element {
  const { t } = useTranslation();
  return (
    <StateShell
      icon={<AlertTriangle className="h-10 w-10" />}
      title={message ?? t("common.errorTitle")}
      description={description ?? t("common.errorDefault")}
      action={
        onRetry ? (
          <Button variant="outline" size="sm" onClick={onRetry}>
            <RefreshCw className="h-4 w-4" aria-hidden />
            {t("common.retry")}
          </Button>
        ) : null
      }
      className={className}
    >
      {reason ? (
        <p className="mt-3 inline-block rounded bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-600">
          {reason}
        </p>
      ) : null}
    </StateShell>
  );
}

/** 403 无权限。 */
export function ForbiddenState({
  description,
  className,
}: {
  description?: string;
  className?: string;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <StateShell
      icon={<Lock className="h-10 w-10" />}
      title={t("common.forbiddenTitle")}
      description={description ?? t("common.forbiddenDescription")}
      className={className}
    />
  );
}

/** 404 路由/资源不存在。 */
export function NotFoundState({
  description,
  className,
}: {
  description?: string;
  className?: string;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <StateShell
      icon={<Ban className="h-10 w-10" />}
      title={t("common.notFoundTitle")}
      description={description ?? t("common.notFoundDescription")}
      className={className}
    />
  );
}

/** 限流（RATE_LIMITED）。 */
export function RateLimitedState({
  description,
  className,
}: {
  description?: string;
  className?: string;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <StateShell
      icon={<Clock className="h-10 w-10" />}
      title={t("common.rateLimitedTitle")}
      description={description ?? t("common.rateLimitedDescription")}
      className={className}
    />
  );
}

/** 乐观锁冲突（CONCURRENT_MODIFICATION）。 */
export function ConcurrentModificationState({
  description,
  onRefresh,
  className,
}: {
  description?: string;
  onRefresh?: () => void;
  className?: string;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <StateShell
      icon={<AlertTriangle className="h-10 w-10" />}
      title={t("common.concurrentModificationTitle")}
      description={description ?? t("common.concurrentModificationDescription")}
      action={
        onRefresh ? (
          <Button variant="outline" size="sm" onClick={onRefresh}>
            <RefreshCw className="h-4 w-4" aria-hidden />
            {t("common.refresh")}
          </Button>
        ) : null
      }
      className={className}
    />
  );
}
