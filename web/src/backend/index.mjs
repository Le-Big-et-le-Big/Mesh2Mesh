// Serves the built UI, the login flow under /auth and the control plane's
// API under /api.
import { createServer } from "node:http";

import { authRoute } from "./auth-routes.mjs";
import { controlPlane, listenPort, publicApiUrl } from "./config.mjs";
import { send, sendJSON } from "./http.mjs";
import { proxy } from "./proxy.mjs";
import { auth, crossOrigin, currentUser } from "./session.mjs";
import { serveStatic } from "./static.mjs";

async function handle(req, res, url) {
  const method = req.method ?? "GET";
  if (url.pathname.startsWith("/auth/")) {
    return await authRoute(req, res, url);
  }
  if (url.pathname.startsWith("/api/")) {
    const user = currentUser(req);
    if (!user) {
      return sendJSON(res, 401, { error: "not logged in" });
    }
    if (method !== "GET" && crossOrigin(req)) {
      return sendJSON(res, 403, { error: "cross-origin request refused" });
    }
    return await proxy(req, res, url.pathname.slice("/api".length), user);
  }
  if (url.pathname === "/config.json") {
    return sendJSON(res, 200, { publicApiUrl, authEnabled: !!auth });
  }
  if (url.pathname === "/healthz") {
    return sendJSON(res, 200, { status: "ok" });
  }
  if (method !== "GET" && method !== "HEAD") {
    return send(res, 405, "method not allowed", { "Content-Type": "text/plain", Allow: "GET, HEAD" });
  }
  return await serveStatic(res, url.pathname);
}

const server = createServer(async (req, res) => {
  const url = new URL(req.url ?? "/", "http://ui");
  try {
    await handle(req, res, url);
  } catch (err) {
    console.error(`${req.method} ${url.pathname}:`, err);
    if (!res.headersSent) {
      sendJSON(res, 500, { error: "internal error" });
    }
  }
});

server.listen(listenPort, () => {
  console.log(`web UI listening on :${listenPort}, control plane ${controlPlane}, nodes use ${publicApiUrl}`);
  if (!auth) console.warn("AUTH_DISABLED=true: anyone who can reach this port can manage the mesh");
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => server.close(() => process.exit(0)));
}
