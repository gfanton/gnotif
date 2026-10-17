// Fakes the live and relay tests share: a page on https://dapp.example, a
// WebSocket the test drives, and a notification frame.

import { Gnotif } from "../src/gnotif.js";

export const SERVER = "https://gnotif.example";
export const ACCEPTED = '{"type":"optins"}';
export const A = [{ trigger: "0000001", value: "g1bob" }];
export const B = [{ trigger: "0000001", value: "g1alice" }];

export function stubGlobal(name, value) {
  const previous = Object.getOwnPropertyDescriptor(globalThis, name);
  Object.defineProperty(globalThis, name, { value, configurable: true, writable: true });
  return () => {
    if (previous) Object.defineProperty(globalThis, name, previous);
    else delete globalThis[name];
  };
}

// FakeWebSocket records what the client sends and lets a test drive the
// server's side. Like a browser, it reports its own close asynchronously.
export class FakeWebSocket {
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
export function stubLive(t, random = 0.5) {
  FakeWebSocket.instances = [];
  t.after(stubGlobal("WebSocket", FakeWebSocket));
  t.after(stubGlobal("location", { origin: "https://dapp.example", href: "https://dapp.example/app/game" }));
  t.after(stubGlobal("navigator", {}));
  t.mock.timers.enable({ apis: ["setTimeout", "setInterval"] });
  const rnd = { value: random };
  t.mock.method(Math, "random", () => rnd.value);
  return rnd;
}

export const flush = () => new Promise((resolve) => setImmediate(resolve));
export const sockets = () => FakeWebSocket.instances;
export const last = () => FakeWebSocket.instances.at(-1);

export async function connected(t, optins = A, options = {}) {
  const g = new Gnotif({ server: SERVER, ...options });
  await g.setOptins(optins);
  const p = g.connect();
  last().open();
  last().receive(ACCEPTED);
  await p;
  t.after(() => g.disconnect());
  return g;
}

export function notification(link, id = "i1") {
  return { type: "notification", id, trigger: "0000001", value: "g1bob", title: "Your turn", body: "Game 7, turn 1",
    link, height: 7, tx: "tx1", time: "2026-10-16T21:46:08.5Z", attrs: [{ key: "game", value: "7" }] };
}
