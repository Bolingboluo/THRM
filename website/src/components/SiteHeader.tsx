"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { Download, Menu, X } from "lucide-react";
import { useEffect, useState } from "react";
import type { Dict } from "@/content/types";
import { localePath, neutralPath } from "@/lib/paths";
import { SITE } from "@/lib/site";
import { cn } from "@/lib/utils";
import { ThemeToggle } from "./ThemeToggle";
import { Wordmark } from "./Wordmark";
import { buttonClass } from "./ui/button";
import { GithubIcon } from "./ui/icons";

export function SiteHeader({ dict }: { dict: Dict }) {
  const [open, setOpen] = useState(false);
  const [scrolled, setScrolled] = useState(false);
  const pathname = usePathname() || "/";
  const nav = dict.nav;
  const isEn = dict.locale === "en";
  const home = localePath(dict.locale, "/");
  // 切换语言时留在同一页
  const otherLocale = localePath(isEn ? "zh-CN" : "en", neutralPath(pathname));
  const current = localePath(dict.locale, neutralPath(pathname));

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 8);
    onScroll();
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

  const links = [
    { href: `${home}#features`, label: nav.features },
    { href: `${home}#devices`, label: nav.devices },
    { href: `${home}#download`, label: nav.download },
    { href: localePath(dict.locale, "/changelog/"), label: nav.changelog },
    { href: `${home}#faq`, label: nav.faq },
  ];

  return (
    <header
      className={cn(
        "sticky top-0 z-50 border-b transition-colors duration-200",
        scrolled || open ? "border-border/80 bg-background/80 backdrop-blur-xl backdrop-saturate-150" : "border-transparent",
      )}
    >
      <div className="mx-auto flex h-16 max-w-6xl items-center gap-6 px-4 sm:px-6">
        <Link href={home} className="shrink-0 rounded-md" aria-label="THRM">
          <Wordmark className="h-[18px]" />
        </Link>

        <nav className="hidden items-center gap-1 md:flex" aria-label="Primary">
          {links.map((l) => (
            <Link
              key={l.href}
              href={l.href}
              className={cn(
                "rounded-lg px-3 py-2 text-sm transition-colors hover:bg-muted hover:text-foreground",
                current === l.href ? "text-foreground" : "text-muted-foreground",
              )}
            >
              {l.label}
            </Link>
          ))}
        </nav>

        <div className="ml-auto flex items-center gap-1">
          <Link
            href={otherLocale}
            hrefLang={isEn ? "zh-CN" : "en"}
            className="inline-flex h-9 items-center rounded-lg px-2.5 text-sm font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
            aria-label={nav.switchLanguage}
            title={nav.switchLanguage}
          >
            {nav.languageLabel}
          </Link>
          <ThemeToggle labels={nav.theme} />
          <a
            href={SITE.repo}
            className="hidden h-9 w-9 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted hover:text-foreground sm:inline-flex"
            aria-label={nav.github}
            title={nav.github}
          >
            <GithubIcon className="h-4 w-4" />
          </a>
          <Link href={`${home}#download`} className={buttonClass({ size: "sm", className: "ml-2 hidden sm:inline-flex" })}>
            <Download className="h-3.5 w-3.5" />
            {nav.downloadCta}
          </Link>
          <button
            type="button"
            className="inline-flex h-9 w-9 items-center justify-center rounded-lg text-foreground hover:bg-muted md:hidden"
            onClick={() => setOpen((v) => !v)}
            aria-expanded={open}
            aria-controls="mobile-nav"
            aria-label={open ? nav.closeMenu : nav.openMenu}
          >
            {open ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
          </button>
        </div>
      </div>

      {open && (
        <nav id="mobile-nav" className="border-t border-border/80 px-4 pb-4 pt-2 md:hidden" aria-label="Primary">
          {links.map((l) => (
            <Link key={l.href} href={l.href} onClick={() => setOpen(false)} className="block rounded-lg px-3 py-3 text-[15px] hover:bg-muted">
              {l.label}
            </Link>
          ))}
          <a href={SITE.repo} className="flex items-center gap-2 rounded-lg px-3 py-3 text-[15px] hover:bg-muted">
            <GithubIcon className="h-4 w-4" />
            {nav.github}
          </a>
        </nav>
      )}
    </header>
  );
}
