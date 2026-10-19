import type { Metadata, Viewport } from "next";
import "../app/globals.css";
import { harmony } from "../app/fonts";
import type { Dict } from "@/content/types";
import { localePath } from "@/lib/paths";
import { SITE } from "@/lib/site";
import { Providers } from "./Providers";

export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#f6f8fc" },
    { media: "(prefers-color-scheme: dark)", color: "#07090d" },
  ],
};

/** path 是不带语言前缀的页面路径，如 "/"、"/changelog" */
export function buildMetadata(dict: Dict, path: "/" | `/${string}/`, title = dict.meta.title): Metadata {
  return {
    metadataBase: new URL(SITE.url),
    title,
    description: dict.meta.description,
    alternates: {
      canonical: localePath(dict.locale, path),
      languages: { "zh-CN": path, en: localePath("en", path) },
    },
    openGraph: {
      type: "website",
      title,
      description: dict.meta.description,
      siteName: SITE.name,
      locale: dict.htmlLang.replace("-", "_"),
      images: [{ url: "/og.png", width: 1536, height: 1024, alt: "THRM" }],
    },
    twitter: { card: "summary_large_image", title, description: dict.meta.description, images: ["/og.png"] },
  };
}

export function RootShell({ dict, children }: { dict: Dict; children: React.ReactNode }) {
  return (
    <html lang={dict.htmlLang} className={harmony.variable} suppressHydrationWarning>
      <body>
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
