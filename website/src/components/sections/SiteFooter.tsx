import Link from "next/link";
import type { Dict } from "@/content/types";
import { localePath } from "@/lib/paths";
import { SITE } from "@/lib/site";
import { Wordmark } from "../Wordmark";

export function SiteFooter({ dict }: { dict: Dict }) {
  const f = dict.footer;
  const links = [
    { href: SITE.repo, label: f.links.github },
    { href: localePath(dict.locale, "/changelog/"), label: f.links.releases, internal: true },
    { href: SITE.issues, label: f.links.issues },
    { href: SITE.privacy, label: f.links.privacy },
    { href: SITE.signing, label: f.links.signing },
    { href: SITE.license, label: f.links.license },
  ];
  return (
    <footer className="border-t border-border/70">
      <div className="mx-auto flex max-w-6xl flex-col gap-8 px-4 py-12 sm:px-6 md:flex-row md:items-start md:justify-between">
        <div className="max-w-sm">
          <Wordmark className="h-4" />
          <p className="mt-4 text-sm leading-relaxed text-muted-foreground">{f.disclaimer}</p>
          <p className="mt-3 text-sm text-muted-foreground">{f.copyright}</p>
        </div>
        <ul className="grid grid-cols-2 gap-x-10 gap-y-3 text-sm sm:grid-cols-3">
          {links.map((l) => (
            <li key={l.href}>
              {l.internal ? (
                <Link href={l.href} className="text-muted-foreground transition-colors hover:text-foreground">
                  {l.label}
                </Link>
              ) : (
                <a href={l.href} className="text-muted-foreground transition-colors hover:text-foreground">
                  {l.label}
                </a>
              )}
            </li>
          ))}
        </ul>
      </div>
    </footer>
  );
}
