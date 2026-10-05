// Checks a live endpoint against the /v1/stream protocol, from the outside:
// every behaviour here is one a client can observe. Run it against any gnotif
// server, naming a realm that has at least one verified trigger:
//
//   GNOTIF_SERVER=https://gnotif.example GNOTIF_TARGET=gno.land/r/<ns>/pingpong/v0 \
//     node --test e2e/conformance/stream.test.mjs

import { before, test } from "node:test";
import assert from "node:assert/strict";

const server = (process.env.GNOTIF_SERVER ?? "").replace(/\/$/, "");
const target = process.env.GNOTIF_TARGET ?? "";
if (server === "" || target === "") {
  throw new Error("set GNOTIF_SERVER to the base URL of a gnotif server, and GNOTIF_TARGET to a realm with a verified trigger");
}
const streamURL = server.replace(/^http/, "ws") + "/v1/stream";
const MAX_FRAME = 8192;
// test1, a well-known test address.
const ADDRESS = "g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5";

/** @type {{ trigger: string, value: string }} */
let known;

before(async () => {
  const res = await fetch(`${server}/v1/triggers?target=${encodeURIComponent(target)}`);
  assert.equal(res.status, 200);
  const triggers = await res.json();
  assert.ok(triggers.length > 0, `${target} has no verified trigger to opt into`);
  known = { trigger: triggers[0].id, value: triggers[0].param ? ADDRESS : "" };
});

/**
 * Opens a live connection whose frames and close code can be awaited in
 * order.
 */
async function connect(t) {
  const ws = new WebSocket(streamURL);
  const frames = [];
  const waiting = [];
  let code = null;
  const closing = [];
  ws.addEventListener("message", (e) => {
    const w = waiting.shift();
    if (w) w(e.data);
    else frames.push(e.data);
  });
  ws.addEventListener("close", (e) => {
    code = e.code;
    for (const w of closing.splice(0)) w(code);
    for (const w of waiting.splice(0)) w(null);
  });
  await new Promise((resolve, reject) => {
    ws.addEventListener("open", resolve, { once: true });
    ws.addEventListener("error", () => reject(new Error(`cannot open ${streamURL}`)), { once: true });
  });
  t.after(() => ws.close());
  return {
    send: (data) => ws.send(data),
    next: () => (frames.length > 0 ? Promise.resolve(frames.shift()) : new Promise((r) => waiting.push(r))),
    closed: () => (code !== null ? Promise.resolve(code) : new Promise((r) => closing.push(r))),
  };
}

function optins(...set) {
  return JSON.stringify({ type: "optins", optins: set });
}

// padded returns an optins frame for the empty set, spaced out to size bytes.
function padded(size) {
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
