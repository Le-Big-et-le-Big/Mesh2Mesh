import type { QueryClient } from "@tanstack/react-query";
import {
  createRootRouteWithContext,
  createRoute,
  createRouter,
  lazyRouteComponent,
  Outlet,
  redirect,
} from "@tanstack/react-router";

import { meQuery } from "@/api";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { NotFound } from "@/routes/not-found";
import { LandingPage } from "@/routes/landing";
import { LoginPage } from "@/routes/login";

const rootRoute = createRootRouteWithContext<{ queryClient: QueryClient }>()({
  component: () => (
    <TooltipProvider delayDuration={200}>
      <Outlet />
      <Toaster position="bottom-right" />
    </TooltipProvider>
  ),
  notFoundComponent: NotFound,
});

const landingRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/",
  component: LandingPage,
});

const loginRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/login",
  validateSearch: (s: Record<string, unknown>): { redirect?: string; error?: string; signup?: boolean } => ({
    redirect: typeof s.redirect === "string" ? s.redirect : undefined,
    error: typeof s.error === "string" ? s.error : undefined,
    signup: s.signup === true || s.signup === "true" ? true : undefined,
  }),
  component: LoginPage,
});

// The server enforces sessions on /api; this only avoids a page of errors.
const adminRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/admin",
  beforeLoad: async ({ context, location }) => {
    const me = await context.queryClient.ensureQueryData(meQuery);
    if (!me) {
      throw redirect({ to: "/login", search: { redirect: location.href } });
    }
    return { me };
  },
  component: lazyRouteComponent(() => import("@/routes/admin/layout"), "AdminLayout"),
});

const adminIndexRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/",
  beforeLoad: () => {
    throw redirect({ to: "/admin/machines" });
  },
});

const machinesRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/machines",
  validateSearch: (s: Record<string, unknown>): { tenant?: string } => ({
    tenant: typeof s.tenant === "string" ? s.tenant : undefined,
  }),
  component: lazyRouteComponent(() => import("@/routes/admin/machines"), "MachinesPage"),
});

const membersRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/members",
  validateSearch: (s: Record<string, unknown>): { tenant?: string } => ({
    tenant: typeof s.tenant === "string" ? s.tenant : undefined,
  }),
  component: lazyRouteComponent(() => import("@/routes/admin/members"), "MembersPage"),
});

const inviteRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: "/invite/$code",
  beforeLoad: async ({ context, location }) => {
    const me = await context.queryClient.ensureQueryData(meQuery);
    if (!me) {
      throw redirect({ to: "/login", search: { redirect: location.href, signup: true } });
    }
  },
  component: lazyRouteComponent(() => import("@/routes/invite"), "InvitePage"),
});

const tenantsRoute = createRoute({
  getParentRoute: () => adminRoute,
  path: "/tenants",
  validateSearch: (s: Record<string, unknown>): { create?: boolean; join?: boolean } => ({
    create: s.create === true || s.create === "true" ? true : undefined,
    join: s.join === true || s.join === "true" ? true : undefined,
  }),
  component: lazyRouteComponent(() => import("@/routes/admin/tenants"), "TenantsPage"),
});

export function createAppRouter(queryClient: QueryClient) {
  return createRouter({
    routeTree: rootRoute.addChildren([
      landingRoute,
      loginRoute,
      inviteRoute,
      adminRoute.addChildren([adminIndexRoute, machinesRoute, membersRoute, tenantsRoute]),
    ]),
    context: { queryClient },
    defaultPreload: "intent",
    scrollRestoration: true,
  });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof createAppRouter>;
  }
}
