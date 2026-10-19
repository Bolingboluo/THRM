import { RootShell, buildMetadata } from "@/components/RootShell";
export { viewport } from "@/components/RootShell";
import { zh } from "@/content/zh";

export const metadata = buildMetadata(zh, "/");

export default function Layout({ children }: { children: React.ReactNode }) {
  return <RootShell dict={zh}>{children}</RootShell>;
}
