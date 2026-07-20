// i18n key 类型：从 zh.json 派生，便于组件使用 t() 时获得 key 补全。
// 添加新文案时同步更新 zh.json / en.json，本类型自动跟进。
import type zh from "./locales/zh.json";

type Shape = typeof zh;

/** 递归构造 "a.b.c" 形式的 key 路径。 */
type Path<T, P extends string = ""> = T extends object
  ? {
      [K in keyof T & string]: T[K] extends object ? Path<T[K], `${P}${K}.`> : `${P}${K}`;
    }[keyof T & string]
  : never;

export type TranslationKey = Path<Shape>;
