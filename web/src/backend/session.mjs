import { oidc } from "./config.mjs";
import { createAuth } from "./oidc.mjs";

export const auth = oidc ? createAuth(oidc) : null;

const devUser = { sub: "dev", name: "Local developer", email: null, username: "dev" };

export function currentUser(req) {
  return auth ? auth.session(req)?.user : devUser;
}

// Backs up the SameSite=Lax cookie for browsers that do not honour it.
export function crossOrigin(req) {
  return auth && req.headers.origin !== auth.publicOrigin;
}
