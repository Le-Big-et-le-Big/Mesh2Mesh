// Forwards the control plane's /v1/me routes for logged-in users, with the
// shared token and the user's identity.
import { controlPlane, controlPlaneToken } from "./config.mjs";
import { readBody, send, sendJSON } from "./http.mjs";

const upstreamTimeoutMs = 15_000;

const tenant = "\\/v1\\/me\\/tenants\\/[0-9a-fA-F-]{36}";
const apiRoutes = [
  ["GET", /^\/healthz$/],
  ["GET", /^\/v1\/me\/tenants$/],
  ["POST", /^\/v1\/me\/tenants$/],
  ["GET", new RegExp(`^${tenant}$`)],
  ["GET", new RegExp(`^${tenant}/(peers|members)$`)],
  ["DELETE", new RegExp(`^${tenant}/members/[\\w.@-]+$`)],
  ["POST", new RegExp(`^${tenant}/(invites|claim)$`)],
  ["POST", /^\/v1\/me\/invites\/accept$/],
];

export async function proxy(req, res, path, user) {
  const method = req.method ?? "GET";
  if (!apiRoutes.some(([m, re]) => m === method && re.test(path))) {
    return sendJSON(res, 404, { error: "not an API route the web UI forwards" });
  }

  let body;
  if (method === "POST") {
    try {
      body = await readBody(req);
    } catch (err) {
      return sendJSON(res, 413, { error: err.message });
    }
  }

  let upstream;
  try {
    upstream = await fetch(controlPlane + path, {
      method,
      body,
      headers: {
        ...(body ? { "Content-Type": "application/json" } : {}),
        Authorization: `Bearer ${controlPlaneToken}`,
        "X-Mesh2Mesh-User": user.sub,
        "X-Mesh2Mesh-User-Email": user.email ?? "",
        "X-Mesh2Mesh-User-Name": encodeURIComponent(user.name ?? ""),
      },
      signal: AbortSignal.timeout(upstreamTimeoutMs),
    });
  } catch (err) {
    console.error(`${method} ${path}: control plane unreachable:`, err.cause?.message ?? err.message);
    return sendJSON(res, 502, { error: `control plane unreachable at ${controlPlane}` });
  }

  const payload = Buffer.from(await upstream.arrayBuffer());
  send(res, upstream.status, payload, {
    "Content-Type": upstream.headers.get("content-type") ?? "application/json",
    "Cache-Control": "no-store",
  });
}
