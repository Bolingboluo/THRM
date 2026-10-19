import type { Dict } from "@/content/types";
import { Hero } from "./hero/Hero";
import { ReleaseProvider } from "./release";
import { Devices } from "./sections/Devices";
import { Download } from "./sections/Download";
import { Faq } from "./sections/Faq";
import { Features } from "./sections/Features";
import { Pipeline } from "./sections/Pipeline";
import { QuickStart } from "./sections/QuickStart";
import { SiteFooter } from "./sections/SiteFooter";
import { SiteHeader } from "./SiteHeader";

export function HomePage({ dict }: { dict: Dict }) {
  return (
    <ReleaseProvider>
      <SiteHeader dict={dict} />
      <main>
        <Hero dict={dict} />
        <Pipeline dict={dict} />
        <Features dict={dict} />
        <Devices dict={dict} />
        <QuickStart dict={dict} />
        <Download dict={dict} />
        <Faq dict={dict} />
      </main>
      <SiteFooter dict={dict} />
    </ReleaseProvider>
  );
}
