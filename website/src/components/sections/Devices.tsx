import { Bluetooth } from "lucide-react";
import type { Dict } from "@/content/types";
import { asset } from "@/lib/utils";

const DEVICES = [
  { name: "BS1", protocol: "BLE", image: "/devices/bs1.webp" },
  { name: "BS2", protocol: "HID", image: "/devices/bs2.webp" },
  { name: "BS2 Pro", protocol: "HID", image: "/devices/bs2pro.webp" },
  // BS3 / BS3 Pro 与 BS2 / BS2 Pro 外观相同，沿用同一张产品图
  { name: "BS3", protocol: "HID", image: "/devices/bs2.webp" },
  { name: "BS3 Pro", protocol: "HID", image: "/devices/bs2pro.webp" },
] as const;

export function Devices({ dict }: { dict: Dict }) {
  const d = dict.devices;
  return (
    <section id="devices" className="border-y border-border/70 bg-card/60">
      <div className="mx-auto max-w-6xl px-4 py-20 sm:px-6 sm:py-24">
        <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)] lg:items-end">
          <h2 className="text-3xl font-bold tracking-[-0.02em] sm:text-[2.5rem] sm:leading-[1.15]">{d.title}</h2>
          <p className="text-[17px] leading-relaxed text-muted-foreground">{d.lead}</p>
        </div>

        <ul className="mt-12 grid grid-cols-2 gap-3 sm:gap-4 md:grid-cols-3 lg:grid-cols-5">
          {DEVICES.map((dev) => (
            <li key={dev.name} className="flex flex-col overflow-hidden rounded-xl border border-border bg-card shadow-sm shadow-black/5">
              <div className="flex aspect-[4/3] items-center justify-center bg-plate">
                <img src={asset(dev.image)} alt={`Flydigi ${dev.name}`} loading="lazy" className="h-full w-full object-contain mix-blend-multiply" draggable={false} />
              </div>
              <div className="flex flex-1 flex-col p-4">
                <div className="flex items-center justify-between gap-2">
                  <h3 className="font-semibold">{dev.name}</h3>
                  <span className="inline-flex items-center gap-1 rounded-full border border-primary/30 bg-primary/10 px-2 py-0.5 font-mono text-[11px] font-medium text-primary">
                    {dev.protocol === "BLE" && <Bluetooth className="h-3 w-3" />}
                    {dev.protocol}
                  </span>
                </div>
                <p className="mt-2 text-[13px] leading-relaxed text-muted-foreground">{dev.protocol === "BLE" ? d.autoScan : d.pairFirst}</p>
              </div>
            </li>
          ))}
        </ul>
      </div>
    </section>
  );
}
