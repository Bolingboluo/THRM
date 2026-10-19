import { ChevronDown } from "lucide-react";
import type { Dict } from "@/content/types";
import { SITE } from "@/lib/site";

export function Faq({ dict }: { dict: Dict }) {
  const f = dict.faq;
  return (
    <section id="faq" className="mx-auto grid max-w-6xl gap-10 px-4 py-20 sm:px-6 sm:py-28 lg:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
      <div>
        <h2 className="text-3xl font-bold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.15]">{f.title}</h2>
        <a href={SITE.issues} className="mt-4 inline-block text-[15px] text-primary underline-offset-4 hover:underline">
          GitHub Issues
        </a>
      </div>
      <div className="divide-y divide-border border-y border-border">
        {f.items.map((item) => (
          <details key={item.q} className="group">
            <summary className="flex cursor-pointer items-center justify-between gap-4 py-5 text-[17px] font-semibold transition-colors hover:text-primary">
              {item.q}
              <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground transition-transform duration-200 group-open:rotate-180" />
            </summary>
            <div className="max-w-[44rem] space-y-3 pb-6 text-[15px] leading-relaxed text-muted-foreground">
              {item.a.map((p) => (
                <p key={p}>{p}</p>
              ))}
            </div>
          </details>
        ))}
      </div>
    </section>
  );
}
