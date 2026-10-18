// Checks a live endpoint against the /v1/stream protocol, from the outside:
// every behaviour here is one a client can observe. Run it against any gnotif
// server, naming a realm that has at least one verified trigger:
//
//   GNOTIF_SERVER=https://gnotif.example GNOTIF_TARGET=gno.land/r/<ns>/pingpong/v0 \
//     node --test e2e/conformance/stream.test.ts

import { before, test, type TestContext } from "node:test";
import assert from "node:assert/strict";

const server = (process.env["GNOTIF_SERVER"] ?? "").replace(/\/$/, "");
const target = process.env["GNOTIF_TARGET"] ?? "";
if (server === "" || target === "") {
  throw new Error(
    "set GNOTIF_SERVER to the base URL of a gnotif server, and GNOTIF_TARGET to a realm with a verified trigger",
  );
}
const streamURL = server.replace(/^http/, "ws") + "/v1/stream";
const MAX_FRAME = 8192;
// test1, a well-known test address.
const ADDRESS = "g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5";

type Optin = { trigger: string; value: string };

type Connection = {
  send: (data: string | Uint8Array) => void;
  /** the next frame, or null once the connection has closed */
  next: () => Promise<unknown>;
  closed: () => Promise<number>;
};

let known: Optin;

function assertListedTrigger(x: unknown): asserts x is { id: string; param: string } {
  assert.ok(typeof x === "object" && x !== null, "trigger is not an object");
  assert.ok("id" in x && typeof x.id === "string", "trigger has no id");
  assert.ok("param" in x && typeof x.param === "string", "trigger has no param");
}

before(async () => {
  const res = await fetch(`${server}/v1/triggers?target=${encodeURIComponent(target)}`);
  assert.equal(res.status, 200);
  const triggers: unknown = await res.json();
  assert.ok(Array.isArray(triggers), "triggers is not an array");
  const first: unknown = triggers[0];
  assert.ok(first !== undefined, `${target} has no verified trigger to opt into`);
  assertListedTrigger(first);
  known = { trigger: first.id, value: first.param !== "" ? ADDRESS : "" };
});

/**
 * Opens a live connection whose frames and close code can be awaited in
 * order.
 */
async function connect(t: TestContext): Promise<Connection> {
  const ws = new WebSocket(streamURL);
  const frames: unknown[] = [];
  const waiting: ((frame: unknown) => void)[] = [];
  let code: number | null = null;
  const closing: ((code: number) => void)[] = [];
  ws.addEventListener("message", (e) => {
    const frame: unknown = e.data;
    const w = waiting.shift();
    if (w !== undefined) w(frame);
    else frames.push(frame);
  });
  ws.addEventListener("close", (e) => {
    code = e.code;
    for (const w of closing.splice(0)) w(e.code);
    for (const w of waiting.splice(0)) w(null);
  });
  await new Promise<void>((resolve, reject) => {
    ws.addEventListener("open", () => resolve(), { once: true });
    ws.addEventListener("error", () => reject(new Error(`cannot open ${streamURL}`)), { once: true });
  });
  t.after(() => ws.close());
  return {
    send: (data) => ws.send(data),
    next: () => (frames.length > 0 ? Promise.resolve(frames.shift()) : new Promise((r) => waiting.push(r))),
    closed: () => (code !== null ? Promise.resolve(code) : new Promise((r) => closing.push(r))),
  };
}

function optins(...set: Optin[]): string {
  return JSON.stringify({ type: "optins", optins: set });
}

// padded returns an optins frame for the empty set, spaced out to size bytes.
function padded(size: number): string {
  const head = '{"type":"optins","optins":[]';
  return head + " ".repeat(size - head.length - 1) + "}";
}

const opts = { timeout: 10_000 };

test("answers the exact ping with the exact pong", opts, async (t) => {
  const c = await connect(t);
  c.send('{"type":"ping"}');
  assert.equal(await c.next(), '{"type":"pong"}');
});

test("treats a ping with other spacing as an unknown message type", opts, async (t) => {
  const c = await connect(t);
  c.send('{ "type": "ping" }');
  assert.equal(await c.next(), '{"type":"error","error":"unknown message type"}');
});

test("accepts a set with a known trigger", opts, async (t) => {
  const c = await connect(t);
  c.send(optins(known));
  assert.equal(await c.next(), '{"type":"optins"}');
});

test("refuses an unknown trigger with its message", opts, async (t) => {
  const c = await connect(t);
  c.send(optins({ trigger: "conformance-unknown", value: "" }));
  assert.equal(await c.next(), '{"type":"error","error":"unknown trigger \\"conformance-unknown\\""}');
});

test("accepts a frame of exactly 8192 bytes", opts, async (t) => {
  const c = await connect(t);
  c.send(padded(MAX_FRAME));
  assert.equal(await c.next(), '{"type":"optins"}');
});

test("closes with 1009 on a frame of 8193 bytes", opts, async (t) => {
  const c = await connect(t);
  c.send(padded(MAX_FRAME + 1));
  assert.equal(await c.closed(), 1009);
});

test("closes with 1003 on a binary frame", opts, async (t) => {
  const c = await connect(t);
  c.send(new Uint8Array([1]));
  assert.equal(await c.closed(), 1003);
});

test("answers 426 without an upgrade", opts, async () => {
  const res = await fetch(server + "/v1/stream");
  assert.equal(res.status, 426);
  assert.deepEqual(await res.json(), { error: "websocket upgrade required" });
});
