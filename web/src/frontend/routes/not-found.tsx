import { Link } from "@tanstack/react-router";

import { LogoMark } from "@/components/logo";
import { Button } from "@/components/ui/button";

export function NotFound() {
  return (
    <div className="flex min-h-[60svh] flex-col items-center justify-center gap-4 px-4 text-center">
      <LogoMark className="size-10" />
      <h1 className="text-xl font-semibold">Page not found</h1>
      <p className="text-muted-foreground">There is nothing at this address.</p>
      <Button variant="outline" asChild>
        <Link to="/">Back home</Link>
      </Button>
    </div>
  );
}
