import { useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, Outlet, useRouteContext } from "@tanstack/react-router";
import {
  BookOpenIcon,
  CheckIcon,
  ChevronsUpDownIcon,
  LogOutIcon,
  MonitorIcon,
  MoonIcon,
  PlusIcon,
  SunIcon,
} from "lucide-react";

import { healthQuery } from "@/api";
import { LogoMark } from "@/components/logo";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useCurrentTenant } from "@/lib/current-tenant";
import { setTheme, useTheme, type Theme } from "@/lib/theme";
import { cn } from "@/lib/utils";

const docsUrl = "https://github.com/Le-Big-et-le-Big/Mesh2Mesh#readme";

export function AdminLayout() {
  return (
    <div className="min-h-svh bg-background">
      <header className="border-b bg-background">
        <div className="mx-auto flex h-14 max-w-6xl items-center justify-between gap-4 px-4 sm:px-6">
          <div className="flex min-w-0 items-center gap-3">
            <Link to="/admin/machines" aria-label="Admin console">
              <LogoMark />
            </Link>
            <span className="text-border select-none" aria-hidden="true">
              /
            </span>
            <TenantSwitcher />
          </div>
          <div className="flex items-center gap-1.5">
            <ControlPlaneStatus />
            <Button variant="ghost" size="sm" className="hidden text-muted-foreground sm:inline-flex" asChild>
              <a href={docsUrl} target="_blank" rel="noreferrer">
                <BookOpenIcon /> Docs
              </a>
            </Button>
            <UserMenu />
          </div>
        </div>
        <AdminTabs />
      </header>
      <main className="mx-auto max-w-6xl px-4 py-8 sm:px-6">
        <Outlet />
      </main>
    </div>
  );
}

function AdminTabs() {
  const tab =
    "relative -mb-px inline-flex h-11 shrink-0 items-center border-b-2 border-transparent px-1 text-sm text-muted-foreground transition-colors hover:text-foreground data-[status=active]:border-foreground data-[status=active]:font-medium data-[status=active]:text-foreground";
  return (
    <nav className="mx-auto flex max-w-6xl gap-6 overflow-x-auto px-4 sm:px-6" aria-label="Admin sections">
      <Link to="/admin/machines" search={(s) => ({ tenant: s.tenant })} className={tab}>
        Machines
      </Link>
      <Link to="/admin/members" search={(s) => ({ tenant: s.tenant })} className={tab}>
        Members
      </Link>
      <Link to="/admin/tenants" className={tab}>
        Tenants
      </Link>
    </nav>
  );
}

function TenantSwitcher() {
  const { tenants, current } = useCurrentTenant();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" className="h-9 min-w-0 gap-2 px-2 font-medium">
          <span className="truncate">{current?.name ?? (tenants.isPending ? "Loading…" : "No tenant")}</span>
          <ChevronsUpDownIcon className="text-muted-foreground" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-64">
        <DropdownMenuLabel className="text-xs text-muted-foreground">Tenants</DropdownMenuLabel>
        {tenants.data?.map((t) => (
          <DropdownMenuItem key={t.id} asChild>
            <Link to="/admin/machines" search={{ tenant: t.id }}>
              <span className="flex min-w-0 flex-1 flex-col">
                <span className="truncate">{t.name}</span>
                <span className="font-mono text-xs text-muted-foreground">{t.cidr}</span>
              </span>
              {t.id === current?.id && <CheckIcon />}
            </Link>
          </DropdownMenuItem>
        ))}
        <DropdownMenuSeparator />
        <DropdownMenuItem asChild>
          <Link to="/admin/tenants" search={{ create: true }}>
            <PlusIcon /> Create tenant
          </Link>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function ControlPlaneStatus() {
  const health = useQuery(healthQuery);
  const up = health.isSuccess;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className="mr-1 hidden items-center gap-2 rounded-full border px-2.5 py-1 text-xs text-muted-foreground md:inline-flex">
          <span
            className={cn("size-1.5 rounded-full", health.isPending ? "bg-border" : up ? "bg-success" : "bg-destructive")}
          />
          {health.isPending ? "Checking" : up ? "Control plane up" : "Control plane down"}
        </span>
      </TooltipTrigger>
      <TooltipContent>{health.isError ? health.error.message : "GET /healthz, every 15 s"}</TooltipContent>
    </Tooltip>
  );
}

function UserMenu() {
  const { me } = useRouteContext({ from: "/admin" });
  const theme = useTheme();
  const logout = useRef<HTMLFormElement>(null);
  const initials = me.name
    .split(/\s+/)
    .map((w) => w[0])
    .join("")
    .slice(0, 2)
    .toUpperCase();

  return (
    <>
      <form ref={logout} method="post" action="/auth/logout" hidden />
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="ghost" size="icon" className="rounded-full" aria-label="Account">
            <Avatar className="size-8">
              <AvatarFallback className="bg-brand text-xs font-semibold text-brand-foreground">{initials}</AvatarFallback>
            </Avatar>
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end" className="w-60">
          <DropdownMenuLabel className="font-normal">
            <div className="truncate text-sm font-medium">{me.name}</div>
            {me.email && <div className="truncate text-xs text-muted-foreground">{me.email}</div>}
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuLabel className="text-xs text-muted-foreground">Theme</DropdownMenuLabel>
          <DropdownMenuRadioGroup value={theme} onValueChange={(v) => setTheme(v as Theme)}>
            <DropdownMenuRadioItem value="light">
              <SunIcon /> Light
            </DropdownMenuRadioItem>
            <DropdownMenuRadioItem value="dark">
              <MoonIcon /> Dark
            </DropdownMenuRadioItem>
            <DropdownMenuRadioItem value="system">
              <MonitorIcon /> System
            </DropdownMenuRadioItem>
          </DropdownMenuRadioGroup>
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={() => logout.current?.requestSubmit()}>
            <LogOutIcon /> Log out
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
    </>
  );
}
