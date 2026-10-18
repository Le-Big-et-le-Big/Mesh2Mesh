import { useState } from "react";
import { useQuery } from "@tanstack/react-query";

import { configQuery, type Peer, type Tenant } from "@/api";
import { Command } from "@/components/copy";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { shellArg } from "@/lib/shell";

const releasesUrl = "https://github.com/Le-Big-et-le-Big/Mesh2Mesh/releases/latest";

// join and run change the node itself, so the UI can only give the commands.
export function AddDeviceDialog({
  tenant,
  open,
  onOpenChange,
}: {
  tenant: Tenant;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const config = useQuery(configQuery);
  const [name, setName] = useState("");
  const [port, setPort] = useState("");
  const [verbose, setVerbose] = useState(false);

  const api = config.data?.publicApiUrl;
  const join = [
    "sudo meshd join",
    tenant.id,
    api ? `--api ${shellArg(api)}` : "",
    name.trim() ? `--name ${shellArg(name.trim())}` : "",
  ]
    .filter(Boolean)
    .join(" ");
  const run = ["sudo meshd run", /^\d{1,5}$/.test(port) ? `--port ${port}` : "", verbose ? "-v" : ""]
    .filter(Boolean)
    .join(" ");

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>Add a Linux server</DialogTitle>
          <DialogDescription>
            Run these on the machine to add it to <span className="font-medium text-foreground">{tenant.name}</span>.
          </DialogDescription>
        </DialogHeader>

        <ol className="space-y-6">
          <Step n={1} title="Install meshd">
            <p className="text-sm text-muted-foreground">
              Download the archive for its architecture from the{" "}
              <a href={releasesUrl} target="_blank" rel="noreferrer" className="text-brand hover:underline">
                latest release
              </a>{" "}
              and put <code className="text-[13px]">meshd</code> on its PATH.
            </p>
          </Step>

          <Step n={2} title="Join the tenant">
            <div className="grid gap-1.5">
              <Label htmlFor="device-name" className="text-[13px] font-normal text-muted-foreground">
                Machine name (defaults to its hostname)
              </Label>
              <Input id="device-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="web-1" />
            </div>
            <Command className="mt-3">{join}</Command>
          </Step>

          <Step n={3} title="Connect">
            <div className="flex flex-wrap items-end gap-4">
              <div className="grid gap-1.5">
                <Label htmlFor="device-port" className="text-[13px] font-normal text-muted-foreground">
                  UDP port (default 51820)
                </Label>
                <Input
                  id="device-port"
                  value={port}
                  onChange={(e) => setPort(e.target.value)}
                  placeholder="51820"
                  inputMode="numeric"
                  className="w-32"
                />
              </div>
              <Label className="h-8 gap-2 text-sm font-normal">
                <Checkbox checked={verbose} onCheckedChange={(v) => setVerbose(v === true)} />
                Verbose logs
              </Label>
            </div>
            <Command className="mt-3">{run}</Command>
            <p className="mt-2 text-[13px] text-muted-foreground">
              It runs as <code>mesh2mesh.service</code> and shows up here as connected within a few seconds.
            </p>
          </Step>
        </ol>
      </DialogContent>
    </Dialog>
  );
}

export function StopDeviceDialog({ peer, onOpenChange }: { peer: Peer | null; onOpenChange: (open: boolean) => void }) {
  return (
    <Dialog open={peer !== null} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Stop {peer?.name}</DialogTitle>
          <DialogDescription>
            Run this on the machine. It stops meshd, disables the service and removes mesh0. The machine stays in the
            tenant: <code>meshd run</code> brings it back.
          </DialogDescription>
        </DialogHeader>
        <Command>sudo meshd stop</Command>
      </DialogContent>
    </Dialog>
  );
}

function Step({ n, title, children }: { n: number; title: string; children: React.ReactNode }) {
  return (
    <li className="grid grid-cols-[28px_1fr] gap-x-3">
      <span className="flex size-7 items-center justify-center rounded-full border text-[13px] font-semibold">{n}</span>
      <div className="min-w-0 pt-0.5">
        <h3 className="mb-2 text-sm font-semibold">{title}</h3>
        {children}
      </div>
    </li>
  );
}
