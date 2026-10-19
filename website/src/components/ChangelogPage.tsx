import type { Dict } from "@/content/types";
import { ReleaseList } from "./ReleaseList";
import { SiteFooter } from "./sections/SiteFooter";
import { SiteHeader } from "./SiteHeader";

export function ChangelogPage({ dict }: { dict: Dict }) {
  const c = dict.changelog;
  return (
    <>
      <SiteHeader dict={dict} />
      <main className="relative">
        <div className="blueprint pointer-events-none absolute inset-x-0 top-0 h-80" aria-hidden="true" />
        <div className="relative mx-auto max-w-4xl px-4 pb-24 pt-14 sm:px-6 sm:pt-20">
          <h1 className="text-4xl font-bold tracking-[-0.03em] sm:text-5xl">{c.title}</h1>
          <p className="mt-4 max-w-2xl text-[17px] leading-relaxed text-muted-foreground">{c.lead}</p>
          <ReleaseList labels={c} dateLocale={dict.htmlLang} />
        </div>
      </main>
      <SiteFooter dict={dict} />
    </>
  );
}
