// A tiny static server for the built site (no dependencies).
//
//   SITE_DIR   the directory to serve (default ../../_site)
//   SITE_PORT  the port (default 8123), always on 127.0.0.1

import { existsSync, readFileSync, statSync } from "node:fs";
import { createServer } from "node:http";
import { extname, join, resolve } from "node:path";

const root = resolve(import.meta.dirname, process.env.SITE_DIR ?? "../../_site");
const PORT = Number(process.env.SITE_PORT ?? 8123);
const HOST = "127.0.0.1";

const TYPES: Record<string, string> = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".wasm": "application/wasm",
  ".woff2": "font/woff2",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".txt": "text/plain; charset=utf-8",
  ".sh": "text/plain; charset=utf-8",
  ".ts": "text/plain; charset=utf-8",
};

const server = createServer((req, res) => {
  let path = "/";
  try {
    path = decodeURIComponent((req.url ?? "/").split("?")[0] ?? "/");
  } catch {
    // keep "/"
  }
  let file = resolve(join(root, path));
  if (file !== root && !file.startsWith(`${root}/`)) {
    res.writeHead(403, { "content-type": "text/plain" });
    res.end("forbidden\n");
    return;
  }
  if (existsSync(file) && statSync(file).isDirectory()) {
    if (!path.endsWith("/")) {
      res.writeHead(301, { location: `${path}/` });
      res.end();
      return;
    }
    file = join(file, "index.html");
  }
  if (!existsSync(file) || !statSync(file).isFile()) {
    res.writeHead(404, { "content-type": "text/plain" });
    res.end("not found\n");
    return;
  }
  res.writeHead(200, {
    "content-type": TYPES[extname(file)] ?? "application/octet-stream",
    "cache-control": "no-store",
  });
  res.end(readFileSync(file));
});
server.on("error", (e) => {
  console.error(e);
  process.exit(1);
});
server.listen(PORT, HOST, () => {
  console.log(`serving ${root} on http://${HOST}:${PORT}/`);
});
