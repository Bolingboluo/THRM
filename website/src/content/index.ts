import { en } from "./en";
import { zh } from "./zh";
import type { Dict, Locale } from "./types";

export const dictionaries: Record<Locale, Dict> = { "zh-CN": zh, en };
export type { Dict, Locale };
