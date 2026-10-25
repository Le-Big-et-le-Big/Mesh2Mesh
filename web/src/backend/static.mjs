import { readFile } from "node:fs/promises";
import { extname, join, normalize } from "node:path";
import { fileURLToPath } from "node:url";

import { send } from "./http.mjs";

const distDir = fileURLToPath(new URL("../../dist/", import.meta.url));

const contentTypes = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".ico": "image/x-icon",
  ".json": "application/json",
  ".woff2": "font/woff2",
};

export async function serveStatic(res, path) {
  const file = join(distDir, normalize(decodeURIComponent(path)));
  if (!file.startsWith(distDir)) {
    return send(res, 404, "not found", { "Content-Type": "text/plain" });
  }
  const ext = extname(file);
  try {
    const data = await readFile(file);
    const immutable = path.startsWith("/assets/");
    return send(res, 200, data, {
      "Content-Type": contentTypes[ext] ?? "application/octet-stream",
      "Cache-Control": immutable ? "public, max-age=31536000, immutable" : "no-cache",
    });
  } catch {
    // Client-side routes fall back to index.html; missing assets stay 404.
    if (ext !== "" && ext !== ".html") {
      return send(res, 404, "not found", { "Content-Type": "text/plain" });
    }
    const index = await readFile(join(distDir, "index.html"));
    return send(res, 200, index, { "Content-Type": contentTypes[".html"], "Cache-Control": "no-cache" });
  }
}
