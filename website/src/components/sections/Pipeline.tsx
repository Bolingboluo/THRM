import { Radio, Spline, Thermometer, Waves } from "lucide-react";
import type { Dict } from "@/content/types";

const ICONS = [Thermometer, Spline, Waves, Radio];

/** 控温链路：四步是真实的先后顺序，所以这里用编号和连线。 */
export function Pipeline({ dict }: { dict: Dict }) {
  const p = dict.pipeline;
  return (
    <section className="border-y border-border/70 bg-card/60">
      <div className="mx-auto max-w-6xl px-4 py-20 sm:px-6 sm:py-24">
        <div className="max-w-2xl">
          <h2 className="text-3xl font-bold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.15]">{p.title}</h2>
          <p className="mt-4 text-[17px] leading-relaxed text-muted-foreground">{p.lead}</p>
        </div>

        <ol className="relative mt-14 grid gap-10 md:grid-cols-4 md:gap-6">
          <div className="absolute left-5 top-5 hidden h-px w-[calc(100%-2.5rem)] bg-gradient-to-r from-primary/60 via-primary/25 to-primary/60 md:block" aria-hidden="true" />
          <div className="absolute bottom-5 left-5 top-5 w-px bg-gradient-to-b from-primary/60 via-primary/25 to-primary/60 md:hidden" aria-hidden="true" />
          {p.steps.map((step, i) => {
            const Icon = ICONS[i];
            return (
              <li key={step.title} className="relative flex gap-5 md:block">
                <span className="relative z-10 flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-primary/25 bg-card text-primary shadow-sm shadow-primary/10">
                  <Icon className="h-[18px] w-[18px]" />
                </span>
                <div className="md:mt-6">
                  <div className="font-mono text-xs text-muted-foreground">{String(i + 1).padStart(2, "0")}</div>
                  <h3 className="mt-1 text-lg font-semibold">{step.title}</h3>
                  <p className="mt-2 text-[15px] leading-relaxed text-muted-foreground">{step.body}</p>
                </div>
              </li>
            );
          })}
        </ol>
      </div>
    </section>
  );
}
