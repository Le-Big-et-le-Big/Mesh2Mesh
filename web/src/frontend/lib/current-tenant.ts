import { useEffect } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearch } from "@tanstack/react-router";

import { tenantsQuery } from "@/api";

const storageKey = "m2m-tenant";

function remembered(): string | null {
  try {
    return localStorage.getItem(storageKey);
  } catch {
    return null;
  }
}

export function useCurrentTenant() {
  const tenants = useQuery(tenantsQuery);
  const { tenant: fromUrl } = useSearch({ strict: false });
  const wanted = fromUrl ?? remembered();
  const current = tenants.data?.find((t) => t.id === wanted) ?? tenants.data?.[0];

  useEffect(() => {
    if (!current) return;
    try {
      localStorage.setItem(storageKey, current.id);
    } catch {}
  }, [current]);

  return { tenants, current };
}
