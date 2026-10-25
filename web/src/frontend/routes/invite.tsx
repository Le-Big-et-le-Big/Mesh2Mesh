import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { toast } from "sonner";
import { UsersIcon } from "lucide-react";

import { acceptInvite, meQuery, tenantsQuery } from "@/api";
import { Logo } from "@/components/logo";
import { Button } from "@/components/ui/button";

export function InvitePage() {
  const { code } = useParams({ from: "/invite/$code" });
  const me = useQuery(meQuery);
  const queryClient = useQueryClient();
  const navigate = useNavigate();

  const accept = useMutation({
    mutationFn: () => acceptInvite(code),
    onSuccess: (tenant) => {
      void queryClient.invalidateQueries({ queryKey: tenantsQuery.queryKey });
      toast.success(`You joined ${tenant.name}`);
      void navigate({ to: "/admin/machines", search: { tenant: tenant.id }, replace: true });
    },
  });

  return (
    <div className="flex min-h-svh flex-col bg-muted/50">
      <header className="px-6 py-5">
        <Link to="/" aria-label="Mesh2Mesh home">
          <Logo />
        </Link>
      </header>
      <main className="flex flex-1 items-start justify-center px-4 pt-[12vh] pb-16">
        <div className="w-full max-w-[400px] rounded-2xl border bg-background p-8 shadow-[0_12px_40px_-20px_rgb(0_0_0/0.25)]">
          <div className="flex size-11 items-center justify-center rounded-xl border bg-muted">
            <UsersIcon className="size-5" />
          </div>
          <h1 className="mt-5 text-2xl font-semibold tracking-tight">You're invited</h1>
          <p className="mt-1.5 text-[15px] text-muted-foreground">
            Join a Mesh2Mesh tenant as <span className="font-medium text-foreground">{me.data?.email ?? me.data?.name}</span>.
            You'll see its machines and can add your own.
          </p>
          {accept.isError && (
            <p role="alert" className="mt-5 rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {accept.error.message}
            </p>
          )}
          <Button size="lg" className="mt-7 h-11 w-full text-[15px]" onClick={() => accept.mutate()} disabled={accept.isPending}>
            {accept.isPending ? "Joining…" : "Accept invite"}
          </Button>
          <Button variant="ghost" className="mt-2 w-full text-muted-foreground" asChild>
            <Link to="/admin/machines">Not now</Link>
          </Button>
        </div>
      </main>
    </div>
  );
}
