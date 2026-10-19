import { RootShell, buildMetadata } from "@/components/RootShell";
export { viewport } from "@/components/RootShell";
import { en } from "@/content/en";

export const metadata = buildMetadata(en, "/");

export default function Layout({ children }: { children: React.ReactNode }) {
  return <RootShell dict={en}>{children}</RootShell>;
}
