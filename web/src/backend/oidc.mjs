// OIDC code flow with PKCE, run server-side: the browser only gets an
// HttpOnly session cookie, never a token.
import { createHash, randomBytes } from "node:crypto";
import { createRemoteJWKSet, jwtVerify } from "jose";

const sessionCookie = "m2m_session";
const loginCookie = "m2m_login";
const loginTimeoutMs = 10 * 60_000;
const discoveryTimeoutMs = 10_000;

export function createAuth(cfg) {
  const issuer = cfg.issuer.replace(/\/+$/, "");
  const discoveryBase = (cfg.internalIssuer ?? issuer).replace(/\/+$/, "");
  const publicUrl = cfg.publicUrl.replace(/\/+$/, "");
  const redirectUri = `${publicUrl}/auth/callback`;
  const secure = publicUrl.startsWith("https://");

  const sessions = new Map();
  const logins = new Map();

  setInterval(() => {
    const now = Date.now();
    for (const [k, v] of sessions) if (v.expires < now) sessions.delete(k);
    for (const [k, v] of logins) if (v.expires < now) logins.delete(k);
  }, 60_000).unref();

  // Lazy, so the UI starts while Keycloak is still booting.
  let metadata;
  async function provider() {
    if (metadata) return metadata;
    const res = await fetch(`${discoveryBase}/.well-known/openid-configuration`, {
      signal: AbortSignal.timeout(discoveryTimeoutMs),
    });
    if (!res.ok) throw new Error(`OIDC discovery: HTTP ${res.status}`);
    const doc = await res.json();
    if (doc.issuer !== issuer) {
      // Keycloak builds the issuer from KC_HOSTNAME; it must be what browsers use.
      throw new Error(`OIDC discovery: issuer is ${doc.issuer}, expected ${issuer} (check KC_HOSTNAME / OIDC_ISSUER)`);
    }
    metadata = { ...doc, jwks: createRemoteJWKSet(new URL(doc.jwks_uri)) };
    return metadata;
  }

  function session(req) {
    const id = readCookie(req, sessionCookie);
    const s = id && sessions.get(id);
    if (!s) return null;
    if (s.expires < Date.now()) {
      sessions.delete(id);
      return null;
    }
    return s;
  }

  async function login(req, res, url) {
    const p = await provider();
    const state = token();
    const verifier = token();
    const nonce = token();
    logins.set(state, { nonce, verifier, returnTo: safeReturnTo(url.searchParams.get("returnTo")), expires: Date.now() + loginTimeoutMs });

    const auth = new URL(p.authorization_endpoint);
    auth.search = new URLSearchParams({
      response_type: "code",
      client_id: cfg.clientId,
      redirect_uri: redirectUri,
      scope: "openid profile email",
      state,
      nonce,
      code_challenge: createHash("sha256").update(verifier).digest("base64url"),
      code_challenge_method: "S256",
      ...(url.searchParams.get("signup") === "1" ? { prompt: "create" } : {}),
    }).toString();

    // Binds the callback to this browser, against login CSRF.
    res.writeHead(302, {
      Location: auth.toString(),
      "Set-Cookie": cookie(loginCookie, state, { path: "/auth", maxAge: loginTimeoutMs / 1000, secure }),
      "Cache-Control": "no-store",
    });
    res.end();
  }

  async function callback(req, res, url) {
    const state = url.searchParams.get("state") ?? "";
    const pending = logins.get(state);
    logins.delete(state);
    const clear = cookie(loginCookie, "", { path: "/auth", maxAge: 0, secure });

    const fail = (reason) => {
      res.writeHead(302, { Location: `/login?error=${encodeURIComponent(reason)}`, "Set-Cookie": clear });
      res.end();
    };

    if (url.searchParams.has("error")) {
      return fail(url.searchParams.get("error_description") ?? url.searchParams.get("error"));
    }
    if (!pending || pending.expires < Date.now() || readCookie(req, loginCookie) !== state) {
      return fail("The login expired or was started in another browser. Please try again.");
    }

    const p = await provider();
    const tokenRes = await fetch(p.token_endpoint, {
      method: "POST",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
        Authorization:
          "Basic " +
          Buffer.from(`${encodeURIComponent(cfg.clientId)}:${encodeURIComponent(cfg.clientSecret)}`).toString("base64"),
      },
      body: new URLSearchParams({
        grant_type: "authorization_code",
        code: url.searchParams.get("code") ?? "",
        redirect_uri: redirectUri,
        code_verifier: pending.verifier,
      }),
      signal: AbortSignal.timeout(discoveryTimeoutMs),
    });
    const tokens = await tokenRes.json().catch(() => ({}));
    if (!tokenRes.ok || !tokens.id_token) {
      console.error("OIDC token exchange failed:", tokenRes.status, tokens.error, tokens.error_description);
      return fail("The identity provider refused the login.");
    }

    let claims;
    try {
      ({ payload: claims } = await jwtVerify(tokens.id_token, p.jwks, { issuer, audience: cfg.clientId }));
    } catch (err) {
      console.error("OIDC id_token rejected:", err.message);
      return fail("The identity provider sent an invalid token.");
    }
    if (claims.nonce !== pending.nonce) {
      return fail("The login response did not match this login.");
    }

    const id = token();
    sessions.set(id, {
      user: {
        sub: claims.sub,
        name: claims.name ?? claims.preferred_username ?? claims.email ?? claims.sub,
        email: claims.email ?? null,
        username: claims.preferred_username ?? null,
      },
      idToken: tokens.id_token,
      expires: Date.now() + cfg.sessionTtlMs,
    });
    console.log(`login: ${claims.preferred_username ?? claims.sub}`);

    res.writeHead(302, {
      Location: pending.returnTo,
      "Set-Cookie": [clear, cookie(sessionCookie, id, { path: "/", maxAge: cfg.sessionTtlMs / 1000, secure })],
      "Cache-Control": "no-store",
    });
    res.end();
  }

  async function logout(req, res) {
    const id = readCookie(req, sessionCookie);
    const s = id && sessions.get(id);
    if (id) sessions.delete(id);

    let location = "/";
    if (s) {
      const p = await provider().catch(() => null);
      if (p?.end_session_endpoint) {
        const end = new URL(p.end_session_endpoint);
        end.search = new URLSearchParams({
          id_token_hint: s.idToken,
          post_logout_redirect_uri: `${publicUrl}/`,
          client_id: cfg.clientId,
        }).toString();
        location = end.toString();
      }
    }
    res.writeHead(303, { Location: location, "Set-Cookie": cookie(sessionCookie, "", { path: "/", maxAge: 0, secure }) });
    res.end();
  }

  return { session, login, callback, logout, publicOrigin: new URL(publicUrl).origin };
}

function safeReturnTo(raw) {
  if (!raw || !raw.startsWith("/") || raw.startsWith("//") || raw.startsWith("/\\")) return "/admin";
  return raw;
}

function token() {
  return randomBytes(32).toString("base64url");
}

function readCookie(req, name) {
  for (const part of (req.headers.cookie ?? "").split(";")) {
    const i = part.indexOf("=");
    if (i > 0 && part.slice(0, i).trim() === name) return part.slice(i + 1).trim();
  }
  return null;
}

function cookie(name, value, { path, maxAge, secure }) {
  return `${name}=${value}; Path=${path}; Max-Age=${Math.floor(maxAge)}; HttpOnly; SameSite=Lax${secure ? "; Secure" : ""}`;
}
