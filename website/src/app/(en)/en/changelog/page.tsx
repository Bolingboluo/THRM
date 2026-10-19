import { ChangelogPage } from "@/components/ChangelogPage";
import { buildMetadata } from "@/components/RootShell";
import { en } from "@/content/en";

export const metadata = buildMetadata(en, "/changelog/", `${en.changelog.title} · THRM`);

export default function Page() {
  return <ChangelogPage dict={en} />;
}
