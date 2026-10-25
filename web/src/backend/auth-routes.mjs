import { redirect, sendJSON } from "./http.mjs";
import { auth, crossOrigin, currentUser } from "./session.mjs";

export async function authRoute(req, res, url) {
  const route = `${req.method ?? "GET"} ${url.pathname}`;
  if (route === "GET /auth/me") {
    return sendJSON(res, 200, currentUser(req) ?? null);
  }
  if (!auth) {
    return redirect(res, 303, route === "GET /auth/login" ? "/admin" : "/");
  }
  switch (route) {
    case "GET /auth/login":
      try {
        return await auth.login(req, res, url);
      } catch (err) {
        console.error("login:", err.message);
        return redirect(res, 302, "/login?error=" + encodeURIComponent("The identity provider is unreachable."));
      }
    case "GET /auth/callback":
      return await auth.callback(req, res, url);
    case "POST /auth/logout":
      if (crossOrigin(req)) return sendJSON(res, 403, { error: "cross-origin request refused" });
      return await auth.logout(req, res);
    default:
      return sendJSON(res, 404, { error: "no such auth route" });
  }
}
