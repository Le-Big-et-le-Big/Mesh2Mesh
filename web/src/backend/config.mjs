const env = process.env;

function fail(message) {
  console.error(message);
  process.exit(1);
}

export const listenPort = Number(env.PORT ?? 8080);
export const controlPlane = (env.CONTROLPLANE_URL ?? "http://localhost:8090").replace(/\/+$/, "");
// CONTROLPLANE_URL is often a container-network name that nodes cannot resolve.
export const publicApiUrl = env.PUBLIC_API_URL ?? controlPlane;

export const controlPlaneToken = env.CONTROLPLANE_TOKEN ?? "";
if (!controlPlaneToken) {
  fail("CONTROLPLANE_TOKEN is not set: it must match the control plane's WEB_API_TOKEN");
}

// Dev only: the API is then open to anyone who can reach this port.
const authDisabled = env.AUTH_DISABLED === "true";
if (!authDisabled && !env.OIDC_ISSUER) {
  fail("OIDC_ISSUER is not set: set OIDC_ISSUER, OIDC_CLIENT_ID, OIDC_CLIENT_SECRET and PUBLIC_URL, or AUTH_DISABLED=true");
}

export const oidc = authDisabled
  ? null
  : {
      issuer: env.OIDC_ISSUER,
      internalIssuer: env.OIDC_INTERNAL_ISSUER || undefined,
      clientId: env.OIDC_CLIENT_ID ?? "mesh2mesh-web",
      clientSecret: env.OIDC_CLIENT_SECRET ?? "",
      publicUrl: env.PUBLIC_URL ?? `http://localhost:${listenPort}`,
      sessionTtlMs: Number(env.SESSION_TTL_HOURS ?? 8) * 3_600_000,
    };
