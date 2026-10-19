import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

/** public/ 下的静态资源路径（统一入口，日后挂子路径时只改这里） */
export function asset(path: string) {
  return path;
}
