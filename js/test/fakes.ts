// Fakes the live and relay tests share: a page on https://dapp.example, a
// WebSocket the test drives, and a notification frame.

import assert from "node:assert/strict";
import type { TestContext } from "node:test";

import { Gnotif, type Optin } from "../src/gnotif.ts";
import type { NotificationFields } from "../src/protocol.ts";

export const SERVER = "https://gnotif.example";
export const ACCEPTED = '{"type":"optins"}';
export const A: Optin[] = [{ trigger: "0000001", value: "g1bob" }];
export const B: Optin[] = [{ trigger: "0000001", value: "g1alice" }];

export function stubGlobal(name: string, value: unknown): () => void {
  const previous = Object.getOwnPropertyDescriptor(globalThis, name);
  Object.defineProperty(globalThis, name, { value, configurable: true, writable: true });
  return () => {
    if (previous !== undefined) Object.defineProperty(globalThis, name, previous);
    else Reflect.deleteProperty(globalThis, name);
  };
}

type Listener = (event: object) => void;

// FakeWebSocket records what the client sends and lets a test drive the
// server's side. Like a browser, it reports its own close asynchronously.
export class FakeWebSocket {
  static OPEN = 1;
  static instances: FakeWebSocket[] = [];

  url: string;
  sent: string[] = [];
  readyState = 0;
  closedWith: number | undefined = undefined;
  listeners: Record<string, Listener[]> = {};

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
  }

  addEventListener(type: string, fn: Listener): void {
    (this.listeners[type] ??= []).push(fn);
  }

  fire(type: string, event: object): void {
    for (const fn of this.listeners[type] ?? []) fn(event);
  }

  send(data: string): void {
    this.sent.push(data);
  }

  close(code?: number): void {
    if (this.readyState === 3) return;
    this.readyState = 3;
    this.closedWith = code ?? 1005;
    queueMicrotask(() => this.fire("close", { code: this.closedWith }));
  }

  open(): void {
    this.readyState = 1;
    this.fire("open", {});
  }

  receive(text: string): void {
    this.fire("message", { data: text });
  }

  drop(code = 1006): void {
    this.readyState = 3;
    this.fire("close", { code });
  }

  optinsSent(): unknown[] {
    const optins: unknown[] = [];
    for (const text of this.sent) {
      const frame: unknown = JSON.parse(text);
      assert.ok(typeof frame === "object" && frame !== null && "type" in frame, text);
      if (frame.type === "optins") {
        assert.ok("optins" in frame, text);
        optins.push(frame.optins);
      }
    }
    return optins;
  }
}

// stubLive gives the client a page on https://dapp.example with no Push API,
// a fake WebSocket and mocked timers.
export function stubLive(t: TestContext, random = 0.5): { value: number } {
  FakeWebSocket.instances = [];
  t.after(stubGlobal("WebSocket", FakeWebSocket));
  t.after(stubGlobal("location", { origin: "https://dapp.example", href: "https://dapp.example/app/game" }));
  t.after(stubGlobal("navigator", {}));
  t.mock.timers.enable({ apis: ["setTimeout", "setInterval"] });
  const rnd = { value: random };
  t.mock.method(Math, "random", () => rnd.value);
  return rnd;
}

export const flush = (): Promise<void> => new Promise((resolve) => setImmediate(resolve));
export const sockets = (): FakeWebSocket[] => FakeWebSocket.instances;

export function last(): FakeWebSocket {
  const ws = FakeWebSocket.instances.at(-1);
  assert.ok(ws !== undefined, "the client opened no socket");
  return ws;
}

export async function connected(
  t: TestContext,
  optins: Optin[] = A,
  options: NonNullable<ConstructorParameters<typeof Gnotif>[0]> = {},
): Promise<Gnotif> {
  const g = new Gnotif({ server: SERVER, ...options });
  await g.setOptins(optins);
  const p = g.connect();
  last().open();
  last().receive(ACCEPTED);
  await p;
  t.after(() => g.disconnect());
  return g;
}

export function notification(link: string, id = "i1"): NotificationFields & { type: "notification" } {
  return {
    type: "notification",
    id,
    trigger: "0000001",
    value: "g1bob",
    title: "Your turn",
    body: "Game 7, turn 1",
    link,
    height: 7,
    tx: "tx1",
    time: "2026-10-16T21:46:08.5Z",
    attrs: [{ key: "game", value: "7" }],
  };
}
