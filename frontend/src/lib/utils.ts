// 通用工具：合并 Tailwind class（shadcn/ui 约定）。
import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

/** 合并 Tailwind utility class，处理冲突（后者覆盖前者）。 */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
