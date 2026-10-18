import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useRouteContext } from "@tanstack/react-router";
import { toast } from "sonner";
import { EllipsisIcon, LogOutIcon, UserMinusIcon, UserPlusIcon } from "lucide-react";

import { createInvite, membersQuery, removeMember, tenantsQuery, type Invite, type Member, type Role, type Tenant } from "@/api";
import { CopyValue, useCopy } from "@/components/copy";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { useCurrentTenant } from "@/lib/current-tenant";
import { formatDate } from "@/lib/time";
import { cn } from "@/lib/utils";
import { NoTenant, TableSkeleton } from "./machines";
import { PageHeader } from "./page-header";

export function MembersPage() {
  const { tenants, current } = useCurrentTenant();
  const [inviting, setInviting] = useState(false);

  if (tenants.isPending) return <TableSkeleton />;
  if (tenants.isError) return <p className="text-sm text-destructive">{tenants.error.message}</p>;
  if (!current) return <NoTenant />;

  const owner = current.role === "owner";
  return (
    <>
      <PageHeader
        title="Members"
        description={
          <>
            The people who can manage <span className="font-medium text-foreground">{current.name}</span>.
          </>
        }
        actions={
          owner && (
            <Button onClick={() => setInviting(true)}>
              <UserPlusIcon /> Invite people
            </Button>
          )
        }
      />
      <MemberList tenant={current} />
      <InviteDialog tenant={current} open={inviting} onOpenChange={setInviting} />
    </>
  );
}

function MemberList({ tenant }: { tenant: Tenant }) {
  const { me } = useRouteContext({ from: "/admin" });
  const members = useQuery(membersQuery(tenant.id));
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const remove = useMutation({
    mutationFn: (m: Member) => removeMember(tenant.id, m.user_id),
    onSuccess: (_, m) => {
      if (m.user_id === me.sub) {
        toast.success(`You left ${tenant.name}`);
        void queryClient.invalidateQueries({ queryKey: tenantsQuery.queryKey });
        void navigate({ to: "/admin/tenants" });
      } else {
        toast.success(`${m.name || m.email} removed`);
        void queryClient.invalidateQueries({ queryKey: membersQuery(tenant.id).queryKey });
      }
    },
    onError: (err) => toast.error(err.message),
  });

  if (members.isPending) return <TableSkeleton />;
  if (members.isError) return <p className="text-sm text-destructive">{members.error.message}</p>;

  const owner = tenant.role === "owner";
  return (
    <div className="overflow-hidden rounded-xl border">
      <Table>
        <TableHeader>
          <TableRow className="bg-muted/50 hover:bg-muted/50">
            {["User", "Role", "Joined", ""].map((h, i) => (
              <TableHead
                key={i}
                className={cn(
                  "h-10 text-[11px] font-medium tracking-wider text-muted-foreground uppercase",
                  i === 0 && "pl-5",
                  i === 2 && "hidden sm:table-cell",
                  i === 3 && "w-12",
                )}
              >
                {h}
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {members.data.map((m) => {
            const self = m.user_id === me.sub;
            return (
              <TableRow key={m.user_id}>
                <TableCell className="py-3 pl-5">
                  <div className="flex items-center gap-3">
                    <Avatar className="size-8">
                      <AvatarFallback className="bg-muted text-xs font-semibold">{initials(m.name || m.email)}</AvatarFallback>
                    </Avatar>
                    <div className="min-w-0">
                      <div className="truncate font-medium">
                        {m.name || m.email || m.user_id}
                        {self && <span className="ml-1.5 font-normal text-muted-foreground">(you)</span>}
                      </div>
                      {m.email && <div className="truncate text-[13px] text-muted-foreground">{m.email}</div>}
                    </div>
                  </div>
                </TableCell>
                <TableCell>
                  <Badge variant={m.role === "owner" ? "default" : "outline"} className="font-normal capitalize">
                    {m.role}
                  </Badge>
                </TableCell>
                <TableCell className="hidden text-muted-foreground sm:table-cell">{formatDate(m.created_at)}</TableCell>
                <TableCell className="pr-3 text-right">
                  {(self || owner) && (
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon-sm" aria-label={`Actions for ${m.name || m.email}`}>
                          <EllipsisIcon />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-48">
                        <DropdownMenuItem variant="destructive" onSelect={() => remove.mutate(m)}>
                          {self ? <LogOutIcon /> : <UserMinusIcon />}
                          {self ? "Leave tenant" : "Remove from tenant"}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  )}
                </TableCell>
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
    </div>
  );
}

const expiries = [
  { label: "1 day", seconds: 86_400 },
  { label: "7 days", seconds: 7 * 86_400 },
  { label: "30 days", seconds: 30 * 86_400 },
];

const audiences = [
  { label: "One person", uses: 1 },
  { label: "Up to 10 people", uses: 10 },
];

function InviteDialog({ tenant, open, onOpenChange }: { tenant: Tenant; open: boolean; onOpenChange: (open: boolean) => void }) {
  const [role, setRole] = useState<Role>("member");
  const [expiry, setExpiry] = useState(expiries[1].seconds);
  const [uses, setUses] = useState(1);
  const [invite, setInvite] = useState<Invite | null>(null);

  const create = useMutation({
    mutationFn: () => createInvite(tenant.id, { role, ttl_seconds: expiry, max_uses: uses }),
    onSuccess: setInvite,
  });

  const close = (o: boolean) => {
    if (!o) {
      setInvite(null);
      create.reset();
    }
    onOpenChange(o);
  };

  return (
    <Dialog open={open} onOpenChange={close}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Invite people to {tenant.name}</DialogTitle>
          <DialogDescription>
            Anyone with the link can create an account, or log in, and join. It is shown only once.
          </DialogDescription>
        </DialogHeader>

        {invite ? (
          <InviteLink invite={invite} />
        ) : (
          <div className="grid gap-5">
            <Choice
              label="Role"
              value={role}
              onChange={setRole}
              options={[
                { value: "member", label: "Member", hint: "Sees machines and adds devices" },
                { value: "owner", label: "Owner", hint: "Also invites and removes people" },
              ]}
            />
            <Choice
              label="Expires after"
              value={expiry}
              onChange={setExpiry}
              options={expiries.map((e) => ({ value: e.seconds, label: e.label }))}
            />
            <Choice
              label="Can be used by"
              value={uses}
              onChange={setUses}
              options={audiences.map((a) => ({ value: a.uses, label: a.label }))}
            />
            {create.isError && (
              <p role="alert" className="rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {create.error.message}
              </p>
            )}
          </div>
        )}

        <DialogFooter>
          {invite ? (
            <Button onClick={() => close(false)}>Done</Button>
          ) : (
            <>
              <Button variant="outline" onClick={() => close(false)}>
                Cancel
              </Button>
              <Button onClick={() => create.mutate()} disabled={create.isPending}>
                {create.isPending ? "Creating…" : "Create invite link"}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function InviteLink({ invite }: { invite: Invite }) {
  const link = `${window.location.origin}/invite/${invite.code}`;
  const [copied, copy] = useCopy();
  return (
    <div className="grid gap-2">
      <div className="flex gap-2">
        <Input readOnly value={link} className="font-mono text-[13px]" onFocus={(e) => e.currentTarget.select()} />
        <Button onClick={() => copy(link)} className="shrink-0">
          {copied ? "Copied" : "Copy link"}
        </Button>
      </div>
      <p className="text-[13px] text-muted-foreground">
        Joins as <span className="font-medium text-foreground">{invite.role}</span>, for{" "}
        {invite.max_uses === 1 ? "one person" : `up to ${invite.max_uses} people`}, until{" "}
        {new Date(invite.expires_at).toLocaleString()}. Or share the code: <CopyValue value={invite.code} display={`${invite.code.slice(0, 12)}…`} />
      </p>
    </div>
  );
}

function Choice<T extends string | number>({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value: T;
  onChange: (v: T) => void;
  options: { value: T; label: string; hint?: string }[];
}) {
  return (
    <fieldset className="grid gap-1.5">
      <legend className="mb-1.5 text-sm font-medium">{label}</legend>
      <div className={cn("grid gap-2", options.length === 3 ? "grid-cols-3" : "grid-cols-2")}>
        {options.map((o) => (
          <button
            key={String(o.value)}
            type="button"
            aria-pressed={o.value === value}
            onClick={() => onChange(o.value)}
            className={cn(
              "rounded-lg border px-3 py-2 text-left text-sm transition-colors hover:bg-muted",
              o.value === value && "border-foreground ring-1 ring-foreground",
            )}
          >
            <div className="font-medium">{o.label}</div>
            {o.hint && <div className="text-[12px] text-muted-foreground">{o.hint}</div>}
          </button>
        ))}
      </div>
    </fieldset>
  );
}

function initials(s: string) {
  return (
    s
      .split(/[\s@.]+/)
      .filter(Boolean)
      .map((w) => w[0])
      .join("")
      .slice(0, 2)
      .toUpperCase() || "?"
  );
}
