import type { Dict } from "@/content/types";

export function QuickStart({ dict }: { dict: Dict }) {
  const q = dict.quickstart;
  return (
    <section className="mx-auto max-w-6xl px-4 py-20 sm:px-6 sm:py-28">
      <h2 className="text-3xl font-bold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.15]">{q.title}</h2>
      <ol className="mt-12 grid gap-px overflow-hidden rounded-xl border border-border bg-border sm:grid-cols-2 lg:grid-cols-4">
        {q.steps.map((s, i) => (
          <li key={s.title} className="bg-card p-6 sm:p-7">
            <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-primary font-mono text-sm font-semibold text-primary-foreground">{i + 1}</span>
            <h3 className="mt-5 text-lg font-semibold">{s.title}</h3>
            <p className="mt-2 text-[15px] leading-relaxed text-muted-foreground">{s.body}</p>
          </li>
        ))}
      </ol>
    </section>
  );
}
