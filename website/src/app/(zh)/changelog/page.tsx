import { ChangelogPage } from "@/components/ChangelogPage";
import { buildMetadata } from "@/components/RootShell";
import { zh } from "@/content/zh";

export const metadata = buildMetadata(zh, "/changelog/", `${zh.changelog.title} · THRM`);

export default function Page() {
  return <ChangelogPage dict={zh} />;
}
