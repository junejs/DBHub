// 页面容器：统一内容区的最大宽度、内边距、标题与操作区。
import type { JSX, ReactNode } from "react";
import { cn } from "@/lib/utils";

export interface PageContainerProps {
  title?: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
}

export function PageContainer({
  title,
  description,
  actions,
  children,
  className,
}: PageContainerProps): JSX.Element {
  return (
    <div className={cn("mx-auto flex w-full max-w-7xl flex-col gap-4 p-6", className)}>
      {(title || actions) && (
        <div className="flex items-start justify-between gap-4">
          <div>
            {title ? <h1 className="text-xl font-semibold text-gray-900">{title}</h1> : null}
            {description ? <p className="mt-1 text-sm text-gray-500">{description}</p> : null}
          </div>
          {actions ? <div className="flex shrink-0 items-center gap-2">{actions}</div> : null}
        </div>
      )}
      <div className="flex-1">{children}</div>
    </div>
  );
}

/** 占位页：未实现的业务页面统一使用此组件，文案走 i18n。 */
export function PlaceholderPage({
  title,
  description,
}: {
  title: string;
  description: string;
}): JSX.Element {
  return (
    <PageContainer title={title}>
      <div className="rounded-lg border border-dashed border-gray-300 bg-white p-10 text-center text-sm text-gray-500">
        {description}
      </div>
    </PageContainer>
  );
}
