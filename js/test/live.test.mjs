import test from "node:test";
import assert from "node:assert/strict";

import { Gnotif, GnotifError } from "../src/gnotif.js";

const SERVER = "https://gnotif.example";
const ACCEPTED = '{"type":"optins"}';
const A = [{ trigger: "0000001", value: "g1bob" }];
const B = [{ trigger: "0000001", value: "g1alice" }];

function stubGlobal(name, value) {
  const previous = Object.getOwnPropertyDescriptor(globalThis, name);
  Object.defineProperty(globalThis, name, { value, configurable: true, writable: true });
  return () => {
    if (previous) Object.defineProperty(globalThis, name, previous);
    else delete globalThis[name];
  };
}

// FakeWebSocket records what the client sends and lets a test drive the
// server's side. Like a browser, it reports its own close asynchronously.
class FakeWebSocket {
  static OPEN = 1;
  static instances = [];

  constructor(url) {
    this.url = url;
    this.sent = [];
    this.readyState = 0;
    this.closedWith = undefined;
    this.listeners = {};
    FakeWebSocket.instances.push(this);
  }

  addEventListener(type, fn) {
    (this.listeners[type] ??= []).push(fn);
  }

  fire(type, event) {
    for (const fn of this.listeners[type] ?? []) fn(event);
  }

  send(data) {
    this.sent.push(data);
  }

  close(code) {
    if (this.readyState === 3) return;
    this.readyState = 3;
    this.closedWith = code ?? 1005;
    queueMicrotask(() => this.fire("close", { code: this.closedWith }));
  }

  open() {
    this.readyState = 1;
    this.fire("open", {});
  }

  receive(text) {
    this.fire("message", { data: text });
  }

  drop(code = 1006) {
    this.readyState = 3;
    this.fire("close", { code });
  }

  optinsSent() {
    return this.sent.map((s) => JSON.parse(s)).filter((f) => f.type === "optins").map((f) => f.optins);
  }
}

// stubLive gives the client a page on https://dapp.example with no Push API,
// a fake WebSocket and mocked timers.
function stubLive(t, random = 0.5) {
  FakeWebSocket.instances = [];
  t.after(stubGlobal("WebSocket", FakeWebSocket));
  t.after(stubGlobal("location", { origin: "https://dapp.example" }));
  t.after(stubGlobal("navigator", {}));
  t.mock.timers.enable({ apis: ["setTimeout", "setInterval"] });
  const rnd = { value: random };
  t.mock.method(Math, "random", () => rnd.value);
  return rnd;
}

const flush = () => new Promise((resolve) => setImmediate(resolve));
const sockets = () => FakeWebSocket.instances;
const last = () => FakeWebSocket.instances.at(-1);

async function connected(t, optins = A) {
  const g = new Gnotif({ server: SERVER });
  await g.setOptins(optins);
  const p = g.connect();
  last().open();
  last().receive(ACCEPTED);
  await p;
  t.after(() => g.disconnect());
  return g;
}

function notification(link) {
  return { type: "notification", trigger: "0000001", value: "g1bob", title: "Your turn", body: "Game 7, turn 1",
    link, height: 7, tx: "tx1", attrs: [{ key: "game", value: "7" }] };
}

test("connect opens the stream and resolves once the set is accepted", async (t) => {
  stubLive(t);
  const g = new Gnotif({ server: SERVER });
  await g.setOptins(A);
  const p = g.connect();
  assert.equal(sockets().length, 1);
  assert.equal(last().url, "wss://gnotif.example/v1/stream");
  last().open();
  assert.deepEqual(last().optinsSent(), [A]);
  last().receive(ACCEPTED);
  await p;
  g.disconnect();
});

test("connect rejects with the server's message and does not retry", async (t) => {
  stubLive(t);
  const g = new Gnotif({ server: SERVER });
  await g.setOptins(A);
  const p = g.connect();
  last().open();
  last().receive('{"type":"error","error":"unknown trigger \\"0000001\\""}');
  await assert.rejects(p, (err) => err instanceof GnotifError && err.code === "server" && err.message === 'unknown trigger "0000001"');
  await flush();
  t.mock.timers.tick(60_000);
  assert.equal(sockets().length, 1);
});

// ThrowingWebSocket refuses its URL at once, as a browser does for ws: from
// an https: page, while throwing.on is set.
class ThrowingWebSocket extends FakeWebSocket {
  static on = true;

  constructor(url) {
    if (ThrowingWebSocket.on) {
      throw new DOMException("refused by the browser", "SecurityError");
    }
    super(url);
  }
}

test("connect rejects with code server when the socket cannot be created, and can be tried again", async (t) => {
  stubLive(t);
  ThrowingWebSocket.on = true;
  t.after(stubGlobal("WebSocket", ThrowingWebSocket));
  const g = new Gnotif({ server: SERVER });
  await assert.rejects(g.connect(), (err) => err instanceof GnotifError && err.code === "server" && err.message === "refused by the browser");

  ThrowingWebSocket.on = false;
  const p = g.connect();
  assert.equal(sockets().length, 1);
  last().open();
  last().receive(ACCEPTED);
  await p;
  g.disconnect();
});

test("a socket refused while reconnecting is retried later", async (t) => {
  stubLive(t, 0);
  t.after(stubGlobal("WebSocket", ThrowingWebSocket));
  ThrowingWebSocket.on = false;
  const g = await connected(t);
  ThrowingWebSocket.on = true;
  last().drop();
  t.mock.timers.tick(500); // the retry is refused at once
  assert.equal(sockets().length, 1);
  ThrowingWebSocket.on = false;
  t.mock.timers.tick(1000);
  assert.equal(sockets().length, 2);
  last().open();
  last().receive(ACCEPTED);
  assert.deepEqual(last().optinsSent(), [A]);
  g.disconnect();
});

test("connect called twice opens one socket", async (t) => {
  stubLive(t);
  const g = new Gnotif({ server: SERVER });
  const p1 = g.connect();
  const p2 = g.connect();
  assert.equal(sockets().length, 1);
  last().open();
  assert.deepEqual(last().optinsSent(), [[]]);
  last().receive(ACCEPTED);
  await Promise.all([p1, p2]);
  g.disconnect();
});

test("disconnect during a pending connect rejects it as inactive", async (t) => {
  stubLive(t);
  const g = new Gnotif({ server: SERVER });
  const p = g.connect();
  last().open();
  g.disconnect();
  await assert.rejects(p, (err) => err instanceof GnotifError && err.code === "inactive");
});

test("notifications reach handlers with the link resolved on the page origin", async (t) => {
  stubLive(t);
  const g = await connected(t);
  const got = [];
  g.on("notification", (n) => got.push(n));
  last().receive(JSON.stringify(notification("/?game=7")));
  last().receive(JSON.stringify(notification("//evil.example/x")));
  const { type, ...fields } = notification("https://dapp.example/?game=7");
  assert.equal(type, "notification");
  assert.deepEqual(got[0], fields);
  assert.equal(got[1].link, "https://dapp.example/");
});

test("on returns a function that removes the handler", async (t) => {
  stubLive(t);
  const g = await connected(t);
  let calls = 0;
  const off = g.on("notification", () => calls++);
  off();
  last().receive(JSON.stringify(notification("/")));
  assert.equal(calls, 0);
});

test("a throwing handler does not stop the others", async (t) => {
  stubLive(t);
  const logged = t.mock.method(console, "error", () => {});
  const g = await connected(t);
  let second = 0;
  g.on("notification", () => {
    throw new Error("boom");
  });
  g.on("notification", () => second++);
  last().receive(JSON.stringify(notification("/")));
  assert.equal(logged.mock.callCount(), 1);
  assert.equal(second, 1);
});

test("pings every 30 s and reconnects without a pong within 10 s", async (t) => {
  stubLive(t);
  const g = await connected(t);
  let disconnects = 0;
  g.on("disconnect", () => disconnects++);

  t.mock.timers.tick(30_000);
  assert.equal(last().sent.at(-1), '{"type":"ping"}');
  last().receive('{"type":"pong"}');
  t.mock.timers.tick(10_000);
  assert.equal(last().closedWith, undefined);

  t.mock.timers.tick(20_000);
  assert.equal(last().sent.at(-1), '{"type":"ping"}');
  t.mock.timers.tick(10_000);
  assert.notEqual(last().closedWith, undefined);
  await flush();
  assert.equal(disconnects, 1);
});

test("reconnects with jittered backoff and fires reconnect", async (t) => {
  const rnd = stubLive(t, 0);
  const g = await connected(t);
  let disconnects = 0;
  let reconnects = 0;
  g.on("disconnect", () => disconnects++);
  g.on("reconnect", () => reconnects++);

  last().drop();
  assert.equal(disconnects, 1);
  t.mock.timers.tick(499);
  assert.equal(sockets().length, 1);
  t.mock.timers.tick(1);
  assert.equal(sockets().length, 2);

  last().drop(); // the retry fails before it opens
  t.mock.timers.tick(999);
  assert.equal(sockets().length, 2);
  t.mock.timers.tick(1);
  assert.equal(sockets().length, 3);
  assert.equal(disconnects, 1);

  last().open();
  assert.deepEqual(last().optinsSent(), [A]);
  last().receive(ACCEPTED);
  assert.equal(reconnects, 1);

  rnd.value = 1;
  last().drop();
  t.mock.timers.tick(999);
  assert.equal(sockets().length, 3);
  t.mock.timers.tick(1);
  assert.equal(sockets().length, 4);
});

test("a set refused after a reconnect fires error", async (t) => {
  stubLive(t, 0);
  const g = await connected(t);
  const errors = [];
  g.on("error", (err) => errors.push(err));
  last().drop();
  t.mock.timers.tick(500);
  last().open();
  last().receive('{"type":"error","error":"unknown trigger \\"0000001\\""}');
  assert.equal(errors.length, 1);
  assert.ok(errors[0] instanceof GnotifError);
  assert.equal(errors[0].message, 'unknown trigger "0000001"');
});

test("setOptins while reconnecting sends the latest set once", async (t) => {
  stubLive(t, 0);
  const g = await connected(t);
  last().drop();
  const p1 = g.setOptins(A);
  const p2 = g.setOptins(B);
  t.mock.timers.tick(500);
  last().open();
  assert.deepEqual(last().optinsSent(), [B]);
  last().receive(ACCEPTED);
  await Promise.all([p1, p2]);
});

test("one optins frame in flight", async (t) => {
  stubLive(t);
  const g = await connected(t, []);
  const ws = last();
  const pa = g.setOptins(A);
  const pb = g.setOptins(B);
  assert.deepEqual(ws.optinsSent(), [[], A]);
  ws.receive(ACCEPTED);
  await pa;
  assert.deepEqual(ws.optinsSent(), [[], A, B]);
  ws.receive(ACCEPTED);
  await pb;
});

test("disconnect closes and stops reconnecting", async (t) => {
  stubLive(t);
  const g = await connected(t);
  let events = 0;
  g.on("disconnect", () => events++);
  g.on("reconnect", () => events++);
  const ws = last();
  g.disconnect();
  assert.equal(ws.closedWith, 1000);
  await flush();
  t.mock.timers.tick(60_000);
  assert.equal(sockets().length, 1);
  assert.equal(events, 0);
});

test("unknown frame types are ignored", async (t) => {
  stubLive(t);
  const g = await connected(t);
  let calls = 0;
  g.on("notification", () => calls++);
  g.on("error", () => calls++);
  last().receive('{"type":"future"}');
  assert.equal(calls, 0);
});
