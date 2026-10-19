import { asset, cn } from "@/lib/utils";

/** 字标有浅/深两版，斜切色块分别是冰川蓝和熔岩橙，跟随主题切换。 */
export function Wordmark({ className }: { className?: string }) {
  return (
    <span className={cn("relative inline-block aspect-[920/200]", className)}>
      <img src={asset("/brand/wordmark-light.webp")} alt="THRM" width={920} height={200} className="h-full w-full dark:hidden" draggable={false} />
      <img src={asset("/brand/wordmark-dark.webp")} alt="" aria-hidden="true" width={920} height={200} className="hidden h-full w-full dark:block" draggable={false} />
    </span>
  );
}
