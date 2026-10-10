import { createServer } from "node:http";
import { readFile, stat } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

/** 启动静态预览服务器；参数含输出目录、端口和挂载前缀，返回 server；测试可用随机端口并在结束后关闭。 */
export function serve({ directory = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../dist"), port = 4321, mount = "/" } = {}) {
  if (!mount.startsWith("/") || !mount.endsWith("/")) throw new Error("挂载路径必须以斜线开始和结束");
  const root = path.resolve(directory);
  const types = { ".html": "text/html; charset=utf-8", ".css": "text/css", ".js": "text/javascript", ".json": "application/json", ".svg": "image/svg+xml", ".png": "image/png" };
  const server = createServer(async (request, response) => {
    try {
      const url = new URL(request.url, "http://localhost");
      const pathname = decodeURIComponent(url.pathname);
      if (!pathname.startsWith(mount) || !["GET", "HEAD"].includes(request.method)) { response.writeHead(404); response.end("Not found"); return; }
      let file = path.resolve(root, pathname.slice(mount.length));
      if (file !== root && !file.startsWith(`${root}${path.sep}`)) { response.writeHead(403); response.end("Forbidden"); return; }
      if ((await stat(file)).isDirectory()) file = path.join(file, "index.html");
      const data = await readFile(file);
      response.writeHead(200, { "Content-Type": types[path.extname(file)] || "application/octet-stream", "Cache-Control": "no-store" });
      response.end(request.method === "HEAD" ? undefined : data);
    } catch { response.writeHead(404); response.end("Not found"); }
  });
  server.listen(port, "127.0.0.1");
  return server;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const port = Number(process.env.XHUB_SITE_PORT || 4321);
  const mount = process.env.XHUB_SITE_MOUNT || "/";
  const server = serve({ port, mount });
  server.on("listening", () => console.log(`XHub website: http://127.0.0.1:${port}${mount}`));
}
