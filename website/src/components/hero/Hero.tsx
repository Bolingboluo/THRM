import { Fragment } from "react";
import { MousePointerClick } from "lucide-react";
import type { Dict } from "@/content/types";
import { AppWindow } from "./AppWindow";
import { HeroActions, ReleaseChip } from "./HeroActions";

export function Hero({ dict }: { dict: Dict }) {
  const h = dict.hero;
  return (
    <section className="relative -mt-16 overflow-hidden pt-16">
      <div className="hero-glow pointer-events-none absolute inset-0" aria-hidden="true" />
      <div className="blueprint pointer-events-none absolute inset-0" aria-hidden="true" />

      <div className="relative mx-auto max-w-6xl px-4 pb-20 pt-10 sm:px-6 sm:pt-16 lg:pb-28">
        <ReleaseChip template={h.release} locale={dict.locale} />

        <h1 className="mt-6 text-[clamp(2.75rem,9vw,5.5rem)] font-bold leading-[1.02] tracking-[-0.035em]">
          {h.title.map((word, i) => (
            <Fragment key={word}>
              {i > 0 && (
                <span className="mx-[0.18em] inline-block text-primary" aria-hidden="true">
                  ·
                </span>
              )}
              <span className="inline-block whitespace-nowrap">{word}</span>
            </Fragment>
          ))}
        </h1>

        <div className="mt-7 grid gap-8 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end">
          <div>
            <span className="slash ml-[0.2em] text-[2.6rem]" aria-hidden="true" />
            <p className="mt-4 max-w-[38rem] text-[17px] leading-relaxed text-muted-foreground sm:text-lg">{h.lead}</p>
          </div>
          <HeroActions labels={h} />
        </div>

        <div className="relative mt-14 sm:mt-16">
          <AppWindow labels={dict.window} />
          <p className="mt-4 flex items-center justify-center gap-2 text-sm text-muted-foreground">
            <MousePointerClick className="h-4 w-4 text-[var(--chart-temperature-indicator)]" />
            {h.demoHint}
          </p>
        </div>
      </div>
    </section>
  );
}
