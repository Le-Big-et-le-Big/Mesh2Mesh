import { Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import {
  ArrowRightIcon,
  BuildingIcon,
  KeyRoundIcon,
  LockIcon,
  RouteIcon,
  ServerIcon,
  TerminalIcon,
} from "lucide-react";

import { meQuery } from "@/api";
import { Logo } from "@/components/logo";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

const repoUrl = "https://github.com/Le-Big-et-le-Big/Mesh2Mesh";

export function LandingPage() {
  return (
    <div className="min-h-svh bg-background">
      <SiteNav />
      <main>
        <Hero />
        <Features />
        <HowItWorks />
        <CallToAction />
      </main>
      <SiteFooter />
    </div>
  );
}

function SiteNav() {
  const me = useQuery(meQuery);
  return (
    <header className="sticky top-0 z-40 border-b bg-background/85 backdrop-blur">
      <div className="mx-auto flex h-16 max-w-6xl items-center justify-between px-4 sm:px-6">
        <Link to="/" aria-label="Mesh2Mesh home">
          <Logo />
        </Link>
        <nav className="hidden items-center gap-7 text-sm text-muted-foreground md:flex">
          <a href="#features" className="hover:text-foreground">
            Features
          </a>
          <a href="#how-it-works" className="hover:text-foreground">
            How it works
          </a>
          <a href={repoUrl} className="hover:text-foreground">
            GitHub
          </a>
        </nav>
        <div className="flex items-center gap-2">
          {me.data ? (
            <Button asChild>
              <Link to="/admin/machines">Admin console</Link>
            </Button>
          ) : (
            <>
              <Button variant="ghost" asChild>
                <Link to="/login">Log in</Link>
              </Button>
              <Button asChild>
                <Link to="/login" search={{ signup: true }}>Get started</Link>
              </Button>
            </>
          )}
        </div>
      </div>
    </header>
  );
}

function Hero() {
  return (
    <section className="relative overflow-hidden">
      <div
        aria-hidden="true"
        className="pointer-events-none absolute inset-x-0 top-0 h-[520px] bg-[radial-gradient(60%_60%_at_50%_0%,color-mix(in_oklab,var(--brand)_14%,transparent),transparent)]"
      />
      <div className="relative mx-auto max-w-6xl px-4 pt-20 pb-16 text-center sm:px-6 sm:pt-28">
        <p className="mx-auto mb-6 inline-flex items-center gap-2 rounded-full border bg-background px-3 py-1 text-[13px] text-muted-foreground">
          <span className="size-1.5 rounded-full bg-success" />
          Self-hosted · WireGuard · Open source
        </p>
        <h1 className="mx-auto max-w-3xl text-4xl font-semibold tracking-tight text-balance sm:text-6xl">
          A private network for every machine you run
        </h1>
        <p className="mx-auto mt-6 max-w-2xl text-lg text-pretty text-muted-foreground">
          Mesh2Mesh gives each node a private address and links them all over WireGuard: directly when it can, through
          NAT hole punching when it has to, and through a relay as a last resort.
        </p>
        <div className="mt-9 flex flex-wrap items-center justify-center gap-3">
          <Button size="lg" className="h-11 px-5 text-[15px]" asChild>
            <Link to="/login" search={{ signup: true }}>
              Get started <ArrowRightIcon />
            </Link>
          </Button>
          <Button size="lg" variant="outline" className="h-11 px-5 text-[15px]" asChild>
            <a href="#how-it-works">See how it works</a>
          </Button>
        </div>
        <ProductPreview />
      </div>
    </section>
  );
}

function ProductPreview() {
  const rows = [
    { name: "web-1", ip: "10.203.0.2", path: "Direct", seen: "Connected" },
    { name: "db-primary", ip: "10.203.0.3", path: "Hole punch", seen: "Connected" },
    { name: "edge-paris", ip: "10.203.0.4", path: "Relayed", seen: "Connected" },
    { name: "build-runner", ip: "10.203.0.5", path: "Direct", seen: "12 min ago" },
  ];
  return (
    <div className="mx-auto mt-16 max-w-4xl rounded-2xl border bg-muted/60 p-2 text-left shadow-[0_24px_60px_-28px_rgb(0_0_0/0.35)]">
      <div className="overflow-hidden rounded-xl border bg-background">
        <div className="flex items-center justify-between border-b px-5 py-3.5">
          <div className="flex items-center gap-2 text-sm font-medium">
            <span className="flex gap-1.5" aria-hidden="true">
              <span className="size-2.5 rounded-full bg-border" />
              <span className="size-2.5 rounded-full bg-border" />
              <span className="size-2.5 rounded-full bg-border" />
            </span>
            <span className="ml-3">Machines</span>
            <span className="text-muted-foreground">· acme</span>
          </div>
          <span className="rounded-md bg-primary px-2.5 py-1 text-xs font-medium text-primary-foreground">
            Add device
          </span>
        </div>
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b text-left text-[11px] tracking-wider text-muted-foreground uppercase">
              <th className="px-5 py-2.5 font-medium">Machine</th>
              <th className="px-5 py-2.5 font-medium">Address</th>
              <th className="hidden px-5 py-2.5 font-medium sm:table-cell">Connection</th>
              <th className="px-5 py-2.5 font-medium">Last seen</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={r.name} className="border-b last:border-0">
                <td className="px-5 py-3 font-medium">{r.name}</td>
                <td className="px-5 py-3 font-mono text-[13px] text-muted-foreground">{r.ip}</td>
                <td className="hidden px-5 py-3 text-muted-foreground sm:table-cell">{r.path}</td>
                <td className="px-5 py-3">
                  <span className="inline-flex items-center gap-2">
                    <span
                      className={cn("size-2 rounded-full", r.seen === "Connected" ? "bg-success" : "bg-border")}
                    />
                    <span className={r.seen === "Connected" ? "" : "text-muted-foreground"}>{r.seen}</span>
                  </span>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

const features = [
  {
    icon: LockIcon,
    title: "Encrypted end to end",
    body: "Every packet on mesh0 is WireGuard. Private keys are made on the node and never leave it.",
  },
  {
    icon: RouteIcon,
    title: "Through any NAT",
    body: "Open port? Peers connect directly. Behind NAT? They hole punch. Symmetric NAT? The relay carries the traffic.",
  },
  {
    icon: ServerIcon,
    title: "Your control plane",
    body: "One Go binary and a Postgres database, on your own servers. No third party in the path of your network.",
  },
  {
    icon: BuildingIcon,
    title: "Isolated tenants",
    body: "Each tenant gets its own private address range. Its nodes see each other, and nothing else.",
  },
  {
    icon: KeyRoundIcon,
    title: "Single sign-on",
    body: "The admin console logs in through Keycloak, so access follows the users and groups you already manage.",
  },
  {
    icon: TerminalIcon,
    title: "Two commands per node",
    body: "Join a tenant, then run. meshd installs itself as a systemd service and keeps its peers current.",
  },
];

function Features() {
  return (
    <section id="features" className="scroll-mt-16 border-t bg-muted/40">
      <div className="mx-auto max-w-6xl px-4 py-24 sm:px-6">
        <div className="max-w-2xl">
          <p className="text-sm font-medium text-brand">Features</p>
          <h2 className="mt-2 text-3xl font-semibold tracking-tight sm:text-4xl">
            Networking that works out of the box
          </h2>
          <p className="mt-4 text-lg text-muted-foreground">
            No firewall rules to write and no ports to forward. Nodes find the best path to each other on their own.
          </p>
        </div>
        <div className="mt-14 grid gap-px overflow-hidden rounded-2xl border bg-border sm:grid-cols-2 lg:grid-cols-3">
          {features.map(({ icon: Icon, title, body }) => (
            <div key={title} className="bg-background p-7">
              <div className="flex size-10 items-center justify-center rounded-lg border bg-muted">
                <Icon className="size-5" />
              </div>
              <h3 className="mt-5 font-semibold">{title}</h3>
              <p className="mt-2 text-[15px] leading-relaxed text-muted-foreground">{body}</p>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

const steps = [
  {
    title: "Create a tenant",
    body: "From the admin console, or from the first node.",
    code: "sudo meshd setup acme",
  },
  {
    title: "Join your machines",
    body: "Each node gets a key and an address from the tenant's range.",
    code: "sudo meshd join <tenant-id> --api https://mesh.example.com",
  },
  {
    title: "Start the mesh",
    body: "meshd brings up mesh0 and connects to every peer.",
    code: "sudo meshd run",
  },
];

function HowItWorks() {
  return (
    <section id="how-it-works" className="scroll-mt-16 border-t">
      <div className="mx-auto max-w-6xl px-4 py-24 sm:px-6">
        <div className="max-w-2xl">
          <p className="text-sm font-medium text-brand">How it works</p>
          <h2 className="mt-2 text-3xl font-semibold tracking-tight sm:text-4xl">Up and running in minutes</h2>
        </div>
        <ol className="mt-14 grid gap-6 lg:grid-cols-3">
          {steps.map((s, i) => (
            <li key={s.title} className="flex min-w-0 flex-col rounded-2xl border p-6">
              <span className="flex size-7 items-center justify-center rounded-full bg-primary text-sm font-semibold text-primary-foreground">
                {i + 1}
              </span>
              <h3 className="mt-5 font-semibold">{s.title}</h3>
              <p className="mt-1.5 text-[15px] text-muted-foreground">{s.body}</p>
              <code className="mt-5 block overflow-x-auto rounded-lg bg-[#1c1c1c] px-4 py-3 text-[13px] whitespace-pre text-[#e8e6e3]">
                <span className="text-[#8a8785] select-none">$ </span>
                {s.code}
              </code>
            </li>
          ))}
        </ol>
      </div>
    </section>
  );
}

function CallToAction() {
  return (
    <section className="border-t">
      <div className="mx-auto max-w-6xl px-4 py-20 sm:px-6">
        <div className="flex flex-col items-start justify-between gap-8 rounded-3xl bg-primary px-8 py-12 text-primary-foreground sm:px-12 md:flex-row md:items-center">
          <div>
            <h2 className="text-2xl font-semibold tracking-tight sm:text-3xl">Your network, on your servers.</h2>
            <p className="mt-2 opacity-70">Log in to the admin console and add your first machine.</p>
          </div>
          <Button size="lg" variant="secondary" className="h-11 px-5 text-[15px]" asChild>
            <Link to="/login">
              Open the console <ArrowRightIcon />
            </Link>
          </Button>
        </div>
      </div>
    </section>
  );
}

function SiteFooter() {
  return (
    <footer className="border-t">
      <div className="mx-auto flex max-w-6xl flex-col items-start justify-between gap-4 px-4 py-10 text-sm text-muted-foreground sm:flex-row sm:items-center sm:px-6">
        <Logo className="[&_svg]:size-6 [&>span]:text-[15px]" />
        <div className="flex gap-6">
          <a href={repoUrl} className="hover:text-foreground">
            GitHub
          </a>
          <a href={`${repoUrl}/blob/main/LICENSE`} className="hover:text-foreground">
            Apache 2.0
          </a>
          <Link to="/login" className="hover:text-foreground">
            Log in
          </Link>
        </div>
      </div>
    </footer>
  );
}
