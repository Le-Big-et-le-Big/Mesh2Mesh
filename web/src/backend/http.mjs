const maxBodyBytes = 64 << 10;

const securityHeaders = {
  "Content-Security-Policy":
    "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'",
  "X-Content-Type-Options": "nosniff",
  "Referrer-Policy": "no-referrer",
};

export function send(res, status, body, headers = {}) {
  res.writeHead(status, { ...securityHeaders, ...headers });
  res.end(body);
}

export function sendJSON(res, status, body) {
  send(res, status, JSON.stringify(body), { "Content-Type": "application/json" });
}

export function redirect(res, status, location) {
  res.writeHead(status, { Location: location });
  res.end();
}

export async function readBody(req) {
  const chunks = [];
  let size = 0;
  for await (const chunk of req) {
    size += chunk.length;
    if (size > maxBodyBytes) {
      throw new Error("request body too large");
    }
    chunks.push(chunk);
  }
  return Buffer.concat(chunks);
}
