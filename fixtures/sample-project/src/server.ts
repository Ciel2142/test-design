import { createServer } from "node:http";
import { createOrder, count } from "./orders.ts";

const failDb = process.env.FAIL_DB === "1";

const server = createServer((req, res) => {
  if (req.url === "/health") {
    res.writeHead(200).end("ok");
    return;
  }
  if (req.method === "POST" && req.url === "/orders") {
    let body = "";
    req.on("data", (c) => (body += c));
    req.on("end", () => {
      let qty: number;
      try {
        qty = JSON.parse(body || "{}").qty;
      } catch {
        res.writeHead(400).end(JSON.stringify({ error: "bad json" }));
        return;
      }
      try {
        const order = createOrder(qty, () => {
          if (failDb) throw new Error("db unavailable");
        });
        res.writeHead(201).end(JSON.stringify(order));
      } catch (err) {
        const message = (err as Error).message;
        const status = message === "db unavailable" ? 503 : 400;
        res.writeHead(status).end(JSON.stringify({ error: message, rows: count() }));
      }
    });
    return;
  }
  res.writeHead(404).end();
});

server.listen(3999, () => console.log("listening on 3999"));
