import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { toast } from "sonner";
import { CopyIcon, EllipsisIcon, PlusIcon, PowerIcon, SearchIcon, ServerIcon } from "lucide-react";

import { peersQuery, type Peer, type Tenant } from "@/api";
import { CopyValue } from "@/components/copy";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useCurrentTenant } from "@/lib/current-tenant";
import { path, pathInfo, presence } from "@/lib/peer";
import { ago } from "@/lib/time";
import { cn } from "@/lib/utils";
import { AddDeviceDialog, StopDeviceDialog } from "./device-dialogs";
import { PageHeader } from "./page-header";

export function MachinesPage() {
  const { tenants, current } = useCurrentTenant();
  const [adding, setAdding] = useState(false);

  if (tenants.isPending) return <TableSkeleton />;
  if (tenants.isError) return <p className="text-sm text-destructive">{tenants.error.message}</p>;
  if (!current) return <NoTenant />;

  return (
    <>
      <PageHeader
        title="Machines"
        description={
          <>
            The nodes in <span className="font-medium text-foreground">{current.name}</span>, on{" "}
            <span className="font-mono text-[13px]">{current.cidr}</span>.
          </>
        }
        actions={
          <Button onClick={() => setAdding(true)}>
            <PlusIcon /> Add device
          </Button>
        }
      />
      <MachineList tenant={current} onAdd={() => setAdding(true)} />
      <AddDeviceDialog tenant={current} open={adding} onOpenChange={setAdding} />
    </>
  );
}

function MachineList({ tenant, onAdd }: { tenant: Tenant; onAdd: () => void }) {
  const peers = useQuery(peersQuery(tenant.id));
  const [filter, setFilter] = useState("");
  const [stopping, setStopping] = useState<Peer | null>(null);

  const shown = useMemo(() => {
    const q = filter.trim().toLowerCase();
    const all = [...(peers.data ?? [])].sort((a, b) => a.name.localeCompare(b.name));
    if (!q) return all;
    return all.filter((p) =>
      [p.name, p.mesh_ip, p.endpoint, p.nat_endpoint, p.peer_id].some((v) => v?.toLowerCase().includes(q)),
    );
  }, [peers.data, filter]);

  if (peers.isPending) return <TableSkeleton />;
  if (peers.isError) return <p className="text-sm text-destructive">{peers.error.message}</p>;
  if (peers.data.length === 0) return <NoMachines onAdd={onAdd} />;

  const connected = peers.data.filter((p) => presence(p) === "connected").length;

  return (
    <>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div className="relative w-full max-w-sm">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Search by name, address or peer ID"
            className="h-9 pl-9"
          />
        </div>
        <p className="text-sm text-muted-foreground">
          {peers.data.length} {peers.data.length === 1 ? "machine" : "machines"} · {connected} connected
        </p>
      </div>

      <div className="overflow-hidden rounded-xl border">
        <Table>
          <TableHeader>
            <TableRow className="bg-muted/50 hover:bg-muted/50">
              <Th className="pl-5">Machine</Th>
              <Th>Address</Th>
              <Th className="hidden md:table-cell">Connection</Th>
              <Th className="hidden lg:table-cell">Endpoint</Th>
              <Th>Last seen</Th>
              <Th className="w-12" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {shown.map((p) => (
              <MachineRow key={p.peer_id} peer={p} onStop={() => setStopping(p)} />
            ))}
            {shown.length === 0 && (
              <TableRow>
                <TableCell colSpan={6} className="py-10 text-center text-muted-foreground">
                  No machine matches “{filter}”.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      <StopDeviceDialog peer={stopping} onOpenChange={(open) => !open && setStopping(null)} />
    </>
  );
}

function Th({ className, children }: { className?: string; children?: React.ReactNode }) {
  return (
    <TableHead className={cn("h-10 text-[11px] font-medium tracking-wider text-muted-foreground uppercase", className)}>
      {children}
    </TableHead>
  );
}

function MachineRow({ peer, onStop }: { peer: Peer; onStop: () => void }) {
  const state = presence(peer);
  const route = path(peer);
  const copy = (what: string, value: string) =>
    navigator.clipboard.writeText(value).then(
      () => toast.success(`${what} copied`),
      () => toast.error("The clipboard needs https or localhost"),
    );

  return (
    <TableRow className="group">
      <TableCell className="py-3.5 pl-5">
        <div className="flex items-center gap-3">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-lg border bg-muted">
            <ServerIcon className="size-4 text-muted-foreground" />
          </div>
          <div className="min-w-0">
            <div className="truncate font-medium">{peer.name}</div>
            <div className="font-mono text-xs text-muted-foreground">{peer.peer_id.slice(0, 8)}</div>
          </div>
        </div>
      </TableCell>
      <TableCell>
        <CopyValue value={peer.mesh_ip} />
      </TableCell>
      <TableCell className="hidden md:table-cell">
        {route && state !== "never" ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <Badge
                variant="outline"
                className={cn(
                  "font-normal",
                  route === "direct" && "border-success/30 text-success",
                  route !== "direct" && "border-warning/30 text-warning",
                )}
              >
                {pathInfo[route].label}
              </Badge>
            </TooltipTrigger>
            <TooltipContent className="max-w-64">{pathInfo[route].hint}</TooltipContent>
          </Tooltip>
        ) : (
          <span className="text-muted-foreground">—</span>
        )}
      </TableCell>
      <TableCell className="hidden font-mono text-[13px] text-muted-foreground lg:table-cell">
        {peer.nat_endpoint ?? peer.endpoint ?? "—"}
      </TableCell>
      <TableCell>
        <LastSeen peer={peer} />
      </TableCell>
      <TableCell className="pr-3 text-right">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label={`Actions for ${peer.name}`}>
              <EllipsisIcon />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-52">
            <DropdownMenuItem onSelect={() => copy("Address", peer.mesh_ip)}>
              <CopyIcon /> Copy address
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => copy("Peer ID", peer.peer_id)}>
              <CopyIcon /> Copy peer ID
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => copy("Public key", peer.public_key)}>
              <CopyIcon /> Copy public key
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={onStop}>
              <PowerIcon /> Stop meshd…
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </TableCell>
    </TableRow>
  );
}

function LastSeen({ peer }: { peer: Peer }) {
  const state = presence(peer);
  if (state === "never") {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <span className="inline-flex items-center gap-2 text-muted-foreground">
            <span className="size-2 rounded-full border border-muted-foreground/50" />
            Not started
          </span>
        </TooltipTrigger>
        <TooltipContent>Joined, but meshd run has not reported in yet</TooltipContent>
      </Tooltip>
    );
  }
  const at = peer.endpoint_updated_at!;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="inline-flex items-center gap-2">
          <span className={cn("size-2 rounded-full", state === "connected" ? "bg-success" : "bg-muted-foreground/40")} />
          <span className={state === "connected" ? "" : "text-muted-foreground"}>
            {state === "connected" ? "Connected" : ago(at)}
          </span>
        </span>
      </TooltipTrigger>
      <TooltipContent>Last report {new Date(at).toLocaleString()}</TooltipContent>
    </Tooltip>
  );
}

function NoMachines({ onAdd }: { onAdd: () => void }) {
  return (
    <div className="flex flex-col items-center rounded-xl border border-dashed px-6 py-16 text-center">
      <div className="flex size-12 items-center justify-center rounded-xl border bg-muted">
        <ServerIcon className="size-5 text-muted-foreground" />
      </div>
      <h2 className="mt-5 font-semibold">Add your first machine</h2>
      <p className="mt-1.5 max-w-sm text-sm text-muted-foreground">
        Join a server to this tenant with meshd. It shows up here as soon as it registers.
      </p>
      <Button className="mt-6" onClick={onAdd}>
        <PlusIcon /> Add device
      </Button>
    </div>
  );
}

export function NoTenant() {
  return (
    <div className="flex flex-col items-center rounded-xl border border-dashed px-6 py-16 text-center">
      <h2 className="font-semibold">Welcome to Mesh2Mesh</h2>
      <p className="mt-1.5 max-w-sm text-sm text-muted-foreground">
        A tenant is a private network with its own address range. Create one, or join one you were invited to.
      </p>
      <div className="mt-6 flex flex-wrap justify-center gap-2">
        <Button variant="outline" asChild>
          <Link to="/admin/tenants" search={{ join: true }}>
            Join with an invite
          </Link>
        </Button>
        <Button asChild>
          <Link to="/admin/tenants" search={{ create: true }}>
            <PlusIcon /> Create tenant
          </Link>
        </Button>
      </div>
    </div>
  );
}

export function TableSkeleton() {
  return (
    <div className="space-y-3">
      <Skeleton className="h-8 w-48" />
      <Skeleton className="h-4 w-80" />
      <div className="mt-6 space-y-2 rounded-xl border p-4">
        {Array.from({ length: 4 }, (_, i) => (
          <Skeleton key={i} className="h-10 w-full" />
        ))}
      </div>
    </div>
  );
}
