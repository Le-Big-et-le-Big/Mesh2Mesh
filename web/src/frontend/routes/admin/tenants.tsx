import { useState, type FormEvent } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { toast } from "sonner";
import { ChevronRightIcon, LogInIcon, PlusIcon } from "lucide-react";

import { acceptInvite, claimTenant, createTenant, tenantsQuery } from "@/api";
import { Badge } from "@/components/ui/badge";
import { Command, CopyValue } from "@/components/copy";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { shellArg } from "@/lib/shell";
import { PageHeader } from "./page-header";
import { TableSkeleton } from "./machines";

export function TenantsPage() {
  const tenants = useQuery(tenantsQuery);
  const { create, join } = useSearch({ from: "/admin/tenants" });
  const navigate = useNavigate({ from: "/admin/tenants" });
  const setCreating = (open: boolean) => void navigate({ search: open ? { create: true } : {}, replace: true });
  const setJoining = (open: boolean) => void navigate({ search: open ? { join: true } : {}, replace: true });

  return (
    <>
      <PageHeader
        title="Tenants"
        description="The private networks you belong to. Each has its own address range and members."
        actions={
          <>
            <Button variant="outline" onClick={() => setJoining(true)}>
              <LogInIcon /> Join a tenant
            </Button>
            <Button onClick={() => setCreating(true)}>
              <PlusIcon /> Create tenant
            </Button>
          </>
        }
      />

      {tenants.isPending ? (
        <TableSkeleton />
      ) : tenants.isError ? (
        <p className="text-sm text-destructive">{tenants.error.message}</p>
      ) : tenants.data.length === 0 ? (
        <div className="rounded-xl border border-dashed px-6 py-16 text-center text-sm text-muted-foreground">
          You are not in any tenant yet. Create one, or join one with an invite link.
        </div>
      ) : (
        <div className="overflow-hidden rounded-xl border">
          <Table>
            <TableHeader>
              <TableRow className="bg-muted/50 hover:bg-muted/50">
                {["Name", "Address range", "Tenant ID", "Your role", ""].map((h, i) => (
                  <TableHead
                    key={i}
                    className={`h-10 text-[11px] font-medium tracking-wider text-muted-foreground uppercase ${i === 0 ? "pl-5" : ""} ${i === 3 ? "hidden md:table-cell" : ""}`}
                  >
                    {h}
                  </TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {tenants.data.map((t) => (
                <TableRow key={t.id}>
                  <TableCell className="py-3.5 pl-5 font-medium">{t.name}</TableCell>
                  <TableCell className="font-mono text-[13px]">{t.cidr}</TableCell>
                  <TableCell>
                    <CopyValue value={t.id} display={`${t.id.slice(0, 8)}…`} />
                  </TableCell>
                  <TableCell className="hidden md:table-cell">
                    <Badge variant={t.role === "owner" ? "default" : "outline"} className="font-normal capitalize">
                      {t.role}
                    </Badge>
                  </TableCell>
                  <TableCell className="pr-3 text-right">
                    <Button variant="ghost" size="sm" asChild>
                      <Link to="/admin/machines" search={{ tenant: t.id }}>
                        Machines <ChevronRightIcon />
                      </Link>
                    </Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <CreateTenantDialog open={!!create} onOpenChange={setCreating} />
      <JoinTenantDialog open={!!join} onOpenChange={setJoining} />
    </>
  );
}

function CreateTenantDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const [name, setName] = useState("");
  const [cidr, setCidr] = useState("");
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const create = useMutation({
    mutationFn: createTenant,
    onSuccess: (tenant) => {
      void queryClient.invalidateQueries({ queryKey: tenantsQuery.queryKey });
      toast.success(`Tenant ${tenant.name} created`);
      setName("");
      setCidr("");
      void navigate({ to: "/admin/machines", search: { tenant: tenant.id } });
    },
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    create.mutate({ name: name.trim(), cidr: cidr.trim() || undefined });
  };

  const cli = `sudo meshd setup ${name.trim() ? shellArg(name.trim()) : "<tenant-name>"}${cidr.trim() ? ` --cidr ${shellArg(cidr.trim())}` : ""}`;

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) create.reset();
        onOpenChange(o);
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={submit} className="grid gap-5">
          <DialogHeader>
            <DialogTitle>Create a tenant</DialogTitle>
            <DialogDescription>A new private network. Machines join it from the Add device dialog.</DialogDescription>
          </DialogHeader>

          <div className="grid gap-1.5">
            <Label htmlFor="tenant-name">Name</Label>
            <Input id="tenant-name" value={name} onChange={(e) => setName(e.target.value)} placeholder="acme" autoFocus required />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="tenant-cidr">
              Address range <span className="font-normal text-muted-foreground">(optional)</span>
            </Label>
            <Input
              id="tenant-cidr"
              value={cidr}
              onChange={(e) => setCidr(e.target.value)}
              placeholder="10.203.0.0/16"
              className="font-mono"
            />
            <p className="text-[13px] text-muted-foreground">A private IPv4 range, /30 or larger.</p>
          </div>

          {create.isError && (
            <p role="alert" className="rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {create.error.message}
            </p>
          )}

          <details className="text-[13px] text-muted-foreground">
            <summary className="cursor-pointer select-none hover:text-foreground">Or from a machine, joining it too</summary>
            <Command className="mt-2">{cli}</Command>
          </details>

          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={create.isPending || name.trim() === ""}>
              {create.isPending ? "Creating…" : "Create tenant"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

const uuid = /[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/i;

function JoinTenantDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const [value, setValue] = useState("");
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const invite = value.match(/m2mi_[\w-]+/)?.[0];
  const tenantId = invite ? undefined : value.match(uuid)?.[0];

  const join = useMutation({
    mutationFn: () => (invite ? acceptInvite(invite) : claimTenant(tenantId!)),
    onSuccess: (tenant) => {
      void queryClient.invalidateQueries({ queryKey: tenantsQuery.queryKey });
      toast.success(invite ? `You joined ${tenant.name}` : `You now own ${tenant.name}`);
      setValue("");
      void navigate({ to: "/admin/machines", search: { tenant: tenant.id } });
    },
  });

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (!o) join.reset();
        onOpenChange(o);
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <form
          className="grid gap-5"
          onSubmit={(e) => {
            e.preventDefault();
            join.mutate();
          }}
        >
          <DialogHeader>
            <DialogTitle>Join a tenant</DialogTitle>
            <DialogDescription>Paste the invite link an owner sent you.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-1.5">
            <Label htmlFor="join-value">Invite link or code</Label>
            <Input
              id="join-value"
              value={value}
              onChange={(e) => setValue(e.target.value)}
              placeholder="https://…/invite/m2mi_…"
              className="font-mono text-[13px]"
              autoFocus
            />
            <p className="text-[13px] text-muted-foreground">
              Made a tenant with <code>meshd setup</code>? Paste its tenant ID to claim it, if nobody has yet.
            </p>
          </div>
          {join.isError && (
            <p role="alert" className="rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {join.error.message}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={join.isPending || (!invite && !tenantId)}>
              {join.isPending ? "Joining…" : tenantId ? "Claim tenant" : "Join tenant"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
