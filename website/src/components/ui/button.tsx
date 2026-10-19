import { cn } from "@/lib/utils";

// 与桌面端 components/ui/button.tsx 的变体保持一致，官网只用到链接形态。
const base =
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-lg font-medium transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background active:scale-[0.98]";

const variants = {
  default: "bg-primary text-primary-foreground shadow-sm shadow-primary/20 hover:opacity-90",
  outline: "border border-input bg-card text-foreground hover:bg-muted",
  ghost: "text-foreground hover:bg-muted",
} as const;

const sizes = {
  sm: "h-8 px-3 text-sm",
  default: "h-9 px-4 text-sm",
  lg: "h-11 px-5 text-[15px]",
} as const;

export type ButtonStyle = { variant?: keyof typeof variants; size?: keyof typeof sizes; className?: string };

export function buttonClass({ variant = "default", size = "default", className }: ButtonStyle = {}) {
  return cn(base, variants[variant], sizes[size], className);
}
