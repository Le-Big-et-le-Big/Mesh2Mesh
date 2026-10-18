import { queryOptions } from "@tanstack/react-query";

export type Role = "owner" | "member";

export type Tenant = {
  id: string;
  name: string;
  cidr: string;
  created_at: string;
  role: Role;
};

export type Member = {
  user_id: string;
  email: string;
  name: string;
  role: Role;
  created_at: string;
};

export type Invite = {
  id: string;
  code: string;
  role: Role;
  max_uses: number;
  expires_at: string;
};

export type Peer = {
  peer_id: string;
  name: string;
  public_key: string;
  mesh_ip: string;
  created_at: string;
  direct_reachable: boolean;
  // Absent until the node has run `meshd run` once.
  endpoint?: string;
  endpoint_updated_at?: string;
  nat_endpoint?: string;
};

export type RuntimeConfig = {
  publicApiUrl: string;
  authEnabled: boolean;
};

export type User = {
  sub: string;
  name: string;
  email: string | null;
  username: string | null;
};

export class ApiError extends Error {
  constructor(
    readonly status: number,
    message: string,
  ) {
    super(message);
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: init?.body ? { "Content-Type": "application/json" } : undefined,
  });
  const body = await res.json().catch(() => null);
  if (!res.ok) {
    throw new ApiError(res.status, body?.error ?? `${res.status} ${res.statusText}`);
  }
  return body as T;
}

// Nodes refresh their peers this often; polling faster shows nothing new.
export const peerRefreshMs = 15_000;

export const meQuery = queryOptions({
  queryKey: ["me"],
  queryFn: () => request<User | null>("/auth/me"),
  staleTime: 60_000,
});

export const healthQuery = queryOptions({
  queryKey: ["health"],
  queryFn: () => request<{ status: string }>("/api/healthz"),
  refetchInterval: peerRefreshMs,
  retry: false,
});

export const configQuery = queryOptions({
  queryKey: ["config"],
  queryFn: () => request<RuntimeConfig>("/config.json"),
  staleTime: Infinity,
});

const me = "/api/v1/me";
const tenantPath = (id: string) => `${me}/tenants/${encodeURIComponent(id)}`;

export const tenantsQuery = queryOptions({
  queryKey: ["tenants"],
  queryFn: async () => (await request<{ tenants: Tenant[] }>(`${me}/tenants`)).tenants,
});

export const peersQuery = (tenantId: string) =>
  queryOptions({
    queryKey: ["tenants", tenantId, "peers"],
    queryFn: async () => (await request<{ peers: Peer[] }>(`${tenantPath(tenantId)}/peers`)).peers,
    refetchInterval: peerRefreshMs,
  });

export const membersQuery = (tenantId: string) =>
  queryOptions({
    queryKey: ["tenants", tenantId, "members"],
    queryFn: async () => (await request<{ members: Member[] }>(`${tenantPath(tenantId)}/members`)).members,
  });

const post = (body: unknown): RequestInit => ({ method: "POST", body: JSON.stringify(body) });

export function createTenant(input: { name: string; cidr?: string }) {
  return request<Tenant>(`${me}/tenants`, post(input.cidr ? input : { name: input.name }));
}

export function createInvite(tenantId: string, input: { role: Role; ttl_seconds: number; max_uses: number }) {
  return request<Invite>(`${tenantPath(tenantId)}/invites`, post(input));
}

export function acceptInvite(code: string) {
  return request<Tenant>(`${me}/invites/accept`, post({ code }));
}

export function claimTenant(tenantId: string) {
  return request<Tenant>(`${tenantPath(tenantId)}/claim`, post({}));
}

export function removeMember(tenantId: string, userId: string) {
  return request<null>(`${tenantPath(tenantId)}/members/${encodeURIComponent(userId)}`, { method: "DELETE" });
}
