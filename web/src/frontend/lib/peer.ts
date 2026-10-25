import type { Peer } from "@/api";

// meshd re-reports its endpoint every 5 minutes while it runs; two missed
// reports means it has most likely stopped.
const offlineAfterMs = 11 * 60_000;

export type Presence = "connected" | "offline" | "never";

export function presence(p: Peer, now = Date.now()): Presence {
  if (!p.endpoint_updated_at) return "never";
  return now - new Date(p.endpoint_updated_at).getTime() > offlineAfterMs ? "offline" : "connected";
}

// Mirrors meshd's own choice from the same fields.
export type Path = "direct" | "hole-punch" | "relay";

export function path(p: Peer): Path | null {
  if (!p.endpoint) return null;
  if (p.direct_reachable) return "direct";
  return p.nat_endpoint ? "hole-punch" : "relay";
}

export const pathInfo: Record<Path, { label: string; hint: string }> = {
  direct: { label: "Direct", hint: "Its UDP port answers the control plane's probe: peers connect straight to it." },
  "hole-punch": {
    label: "Hole punch",
    hint: "Behind NAT: peers punch through to its mapping and fall back to the relay.",
  },
  relay: { label: "Relayed", hint: "Not reachable directly and no NAT mapping: traffic goes through the relay." },
};
