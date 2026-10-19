"use client";

import { Monitor, Moon, Sun } from "lucide-react";
import { useTheme } from "next-themes";
import { useEffect, useState } from "react";
import type { Dict } from "@/content/types";

// 与桌面端一致的三态：跟随系统 → 浅色 → 深色
const ORDER = ["system", "light", "dark"] as const;
type Mode = (typeof ORDER)[number];

export function ThemeToggle({ labels }: { labels: Dict["nav"]["theme"] }) {
  const { theme, setTheme } = useTheme();
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);

  const current: Mode = mounted && (ORDER as readonly string[]).includes(theme ?? "") ? (theme as Mode) : "system";
  const next = ORDER[(ORDER.indexOf(current) + 1) % ORDER.length];
  const Icon = current === "light" ? Sun : current === "dark" ? Moon : Monitor;
  const label = `${labels.label}${labels[current]}`;

  return (
    <button
      type="button"
      onClick={() => setTheme(next)}
      className="inline-flex h-9 w-9 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
      aria-label={label}
      title={label}
    >
      <Icon className="h-4 w-4" />
    </button>
  );
}
