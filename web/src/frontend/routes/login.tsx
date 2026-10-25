import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useNavigate, useSearch } from "@tanstack/react-router";
import { KeyRoundIcon, TriangleAlertIcon } from "lucide-react";

import { configQuery, meQuery } from "@/api";
import { Logo } from "@/components/logo";
import { Button } from "@/components/ui/button";

export function LoginPage() {
  const { redirect, error, signup } = useSearch({ from: "/login" });
  const me = useQuery(meQuery);
  const config = useQuery(configQuery);
  const navigate = useNavigate();

  const returnTo = redirect?.startsWith("/") && !redirect.startsWith("//") ? redirect : "/admin/machines";

  const signIn = `/auth/login?returnTo=${encodeURIComponent(returnTo)}${signup ? "&signup=1" : ""}`;

  // The form is Keycloak's; this page only stays up to show a failed login.
  useEffect(() => {
    if (me.data) void navigate({ to: returnTo, replace: true });
    else if (me.data === null && !error) window.location.replace(signIn);
  }, [me.data, navigate, returnTo, error, signIn]);

  return (
    <div className="flex min-h-svh flex-col bg-muted/50">
      <header className="px-6 py-5">
        <Link to="/" aria-label="Mesh2Mesh home">
          <Logo />
        </Link>
      </header>

      <main className="flex flex-1 items-start justify-center px-4 pt-[12vh] pb-16">
        <div className="w-full max-w-[400px]">
          <div className="rounded-2xl border bg-background p-8 shadow-[0_12px_40px_-20px_rgb(0_0_0/0.25)]">
            <h1 className="text-2xl font-semibold tracking-tight">{error ? "Log in failed" : "Log in"}</h1>
            <p className="mt-1.5 text-[15px] text-muted-foreground">to the Mesh2Mesh admin console</p>

            {error && (
              <div
                role="alert"
                className="mt-6 flex gap-2.5 rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive"
              >
                <TriangleAlertIcon className="mt-0.5 size-4 shrink-0" />
                <span>{error}</span>
              </div>
            )}

            <div className="mt-7 space-y-3">
              <Button asChild size="lg" className="h-11 w-full text-[15px]">
                <a href={signIn}>
                  <KeyRoundIcon />
                  {config.data?.authEnabled === false ? "Continue (login disabled)" : error ? "Try again" : "Continue to sign-in"}
                </a>
              </Button>
            </div>

            <p className="mt-6 text-[13px] leading-relaxed text-muted-foreground">
              You will sign in on your organization's Keycloak, then come back here. Ask your administrator for an
              account if you don't have one.
            </p>
          </div>

          <p className="mt-6 text-center text-sm text-muted-foreground">
            <Link to="/" className="hover:text-foreground">
              ← Back to the home page
            </Link>
          </p>
        </div>
      </main>
    </div>
  );
}
