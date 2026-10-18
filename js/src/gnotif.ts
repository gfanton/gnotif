// gnotif browser client: subscribes this browser to a gnotif server's
// pushes, holds an open page's live connection, and sets which triggers
// both hear about.

import { safeLink } from "./link.ts";
import { type NotificationFields, parseErrorMessage, parseFrame, parseRelay, parseTriggers } from "./protocol.ts";
import { parseVapidKey } from "./push.ts";

/** A trigger as GET /v1/triggers returns it. */
export type Trigger = {
  id: string;
  /** realm path the event comes from */
  target: string;
  event: string;
  /** comma-separated key=value pairs, "" for none */
  filter: string;
  /** name of the parameter a user opts into, "" for none */
  param: string;
  title: string;
  body: string;
  link: string;
  /** address that declared the trigger */
  declarer: string;
  verified: boolean;
};

/** One trigger a browser asks to hear about, for one param value. */
export type Optin = {
  /** trigger id */
  trigger: string;
  /** the param value, "" for a trigger without a param */
  value: string;
  /**
   * false keeps the item off push: the page hears it live only. Push stays
   * on when absent.
   */
  push?: boolean | undefined;
};

/** A notification as either channel delivers it. */
export type GnotifNotification = {
  /** identifies the match; both channels carry the same one */
  id: string;
  /** the channel that delivered it first */
  source: "live" | "push";
  /** trigger id */
  trigger: string;
  /** the opted-in param value, "" for none */
  value: string;
  title: string;
  body: string;
  /** absolute URL on the page's origin */
  link: string;
  /** block height of the event */
  height: number;
  /** hash of the transaction that emitted the event */
  tx: string;
  /** the block's time, in UTC, as RFC 3339 */
  time: string;
  /** the event's attributes, in order */
  attrs: { key: string; value: string }[];
};

/** What each event's handler receives. */
export type GnotifEvents = {
  /** a notification arrived, live or by push */
  notification: GnotifNotification;
  /** the live connection dropped and is retrying */
  reconnecting: undefined;
  /** it is back; notifications in between were missed */
  reconnect: undefined;
  /** a live failure no promise waits on */
  error: GnotifError;
};

const PING = '{"type":"ping"}';
const PING_INTERVAL = 30_000;
const PONG_TIMEOUT = 10_000;
const BACKOFF_MIN = 1_000;
const BACKOFF_MAX = 30_000;
const DELIVERED_MAX = 256;

/** @param scope the service worker's scope */
function browserNotification(fields: NotificationFields, scope: string, source: "live" | "push"): GnotifNotification {
  return { ...fields, source, link: safeLink(fields.link, scope) };
}

export class GnotifError extends Error {
  status: number | undefined;
  code: "unsupported" | "denied" | "inactive" | "server" | "network";

  /** @param status HTTP status of the error answer a "server" error comes from */
  constructor(code: GnotifError["code"], message: string, status?: number) {
    super(message);
    this.status = status;
    this.name = "GnotifError";
    this.code = code;
  }
}

/** @param s unpadded base64url text */
export function base64UrlToBytes(s: string): Uint8Array {
  const b64 = s
    .replaceAll("-", "+")
    .replaceAll("_", "/")
    .padEnd(Math.ceil(s.length / 4) * 4, "=");
  return Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
}

export function sameKey(a: ArrayBuffer | null | undefined, b: Uint8Array): boolean {
  if (a === null || a === undefined) {
    return false;
  }
  const bytes = new Uint8Array(a);
  return bytes.length === b.length && bytes.every((v, i) => v === b[i]);
}

/** The hosted gnotif server of each gno.land network. */
const servers = new Map([["onyx", "https://gnotif.xyz/onyx"]]);

export class Gnotif {
  #network: string | undefined;
  #server: string;
  #serviceWorker: string;
  #icon: string | undefined;
  /** the set every channel uses, null until setOptins */
  #optins: Optin[] | null = null;
  #handlers: { [K in keyof GnotifEvents]: Set<(arg: GnotifEvents[K]) => void> } = {
    notification: new Set(),
    reconnecting: new Set(),
    reconnect: new Set(),
    error: new Set(),
  };
  /** the last DELIVERED_MAX delivered ids, oldest first */
  #delivered = new Set<string>();

  // ---- Live connection state
  #ws: WebSocket | null = null;
  #wanted = false; // connect() was called and disconnect() was not
  #reconnecting = false; // dropped after its set was accepted, and retrying
  #attempt = 0; // retries since the last accepted set
  #connecting: { promise: Promise<void>; resolve: () => void; reject: (err: GnotifError) => void } | null = null;
  #gen = 0; // counts setOptins calls
  #sentGen = 0; // the generation of the optins frame last sent
  #inFlight = false; // an optins frame awaits its reply
  #dirty = false; // the kept set changed while a frame was in flight
  #waiters: { gen: number; resolve: () => void; reject: (err: GnotifError) => void }[] = [];
  #retry: ReturnType<typeof setTimeout> | undefined;
  #pong: ReturnType<typeof setTimeout> | undefined;
  #ping: ReturnType<typeof setInterval> | undefined;

  constructor({
    network,
    server,
    serviceWorker = "/sw.js",
    icon,
  }: {
    /** gno.land network whose hosted server to use, "onyx" by default; only a label when server is set */
    network?: string | undefined;
    /** base URL of another gnotif server, such as your own gnotifd */
    server?: string | undefined;
    /** URL of the dapp's copy of sw.js */
    serviceWorker?: string | undefined;
    /** URL of the image every notification shows, such as a PNG of the dapp's logo, relative to the page */
    icon?: string | undefined;
  } = {}) {
    if (server === undefined) {
      network ??= "onyx";
      server = servers.get(network);
      if (server === undefined) {
        const known = [...servers.keys()].map((name) => `"${name}"`).join(", ");
        throw new TypeError(`unknown network "${network}": use ${known}, or pass server`);
      }
    }
    this.#network = network;
    this.#server = server.replace(/\/$/, "");
    this.#serviceWorker = serviceWorker;
    this.#icon = icon;
    globalThis.navigator?.serviceWorker?.addEventListener?.("message", (e) => this.#onRelay(e.data));
  }

  /** @returns the network named at construction */
  get network(): string | undefined {
    return this.#network;
  }

  /** @returns base URL of the gnotif server this client calls */
  get server(): string {
    return this.#server;
  }

  /**
   * @param target realm path whose verified triggers to list
   * @returns the triggers that realm offers
   */
  async triggers(target: string): Promise<Trigger[]> {
    if (typeof target !== "string" || target === "") {
      throw new TypeError("target must be a non-empty realm path");
    }
    const triggers = parseTriggers(await this.#request("GET", `/v1/triggers?target=${encodeURIComponent(target)}`));
    if (triggers === undefined) {
      throw new GnotifError("server", "gnotif server sent a malformed trigger list");
    }
    return triggers;
  }

  /**
   * Subscribes this browser and registers it with the server. Must run from
   * a user gesture: browsers refuse or hide a permission prompt otherwise.
   */
  async enable(): Promise<PushSubscriptionJSON> {
    if (!("serviceWorker" in navigator) || !("PushManager" in globalThis) || !("Notification" in globalThis)) {
      throw new GnotifError("unsupported", "This browser has no Push API.");
    }
    if ((await Notification.requestPermission()) !== "granted") {
      throw new GnotifError("denied", "Notifications are not allowed for this site.");
    }
    let url = `${this.#serviceWorker}?server=${encodeURIComponent(this.#server)}`;
    if (this.#icon !== undefined) {
      url += `&icon=${encodeURIComponent(new URL(this.#icon, document.baseURI).href)}`;
    }
    await navigator.serviceWorker.register(url);
    const reg = await navigator.serviceWorker.ready;
    const publicKey = parseVapidKey(await this.#request("GET", "/v1/vapid"));
    if (publicKey === undefined) {
      throw new GnotifError("server", "gnotif server sent a malformed VAPID key");
    }
    const key = base64UrlToBytes(publicKey);
    let sub = await reg.pushManager.getSubscription();
    if (sub !== null && !sameKey(sub.options.applicationServerKey, key)) {
      await sub.unsubscribe();
      sub = null;
    }
    sub ??= await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: new Uint8Array(key) });
    const json = sub.toJSON();
    await this.#request("PUT", "/v1/subscription", json);
    if (this.#optins !== null) {
      await this.#request("PUT", "/v1/subscription/optins", { endpoint: sub.endpoint, optins: this.#items(true) });
    }
    return json;
  }

  /**
   * Keeps optins as the set every channel uses, and sends it to the live
   * connection when one is open, and to the server for push when this
   * browser is subscribed, less the items with push: false. Resolves once
   * each has accepted its part.
   * @param optins replaces the previous set
   */
  async setOptins(optins: Optin[]): Promise<void> {
    this.#optins = optins;
    const gen = ++this.#gen;
    const pending: Promise<unknown>[] = [];
    if (this.#wanted && this.#ws?.readyState === WebSocket.OPEN) {
      pending.push(
        new Promise<void>((resolve, reject) => {
          this.#waiters.push({ gen, resolve, reject });
        }),
      );
      this.#send();
    }
    pending.push(
      this.#find().then((sub) =>
        sub !== null
          ? this.#request("PUT", "/v1/subscription/optins", { endpoint: sub.endpoint, optins: this.#items(true) })
          : null,
      ),
    );
    await Promise.all(pending);
  }

  /**
   * Calls handler on every event of that name until the returned function
   * runs.
   * @returns removes the handler
   */
  on<K extends keyof GnotifEvents>(event: K, handler: (arg: GnotifEvents[K]) => void): () => void {
    if (!Object.hasOwn(this.#handlers, event)) {
      const known = Object.keys(this.#handlers).map((name) => `"${name}"`);
      throw new TypeError(`unknown event "${event}": use ${known.slice(0, -1).join(", ")} or ${known.at(-1)}`);
    }
    const handlers = this.#handlers[event];
    handlers.add(handler);
    return () => {
      handlers.delete(handler);
    };
  }

  /**
   * Opens the live connection with the kept set, and resolves once the
   * server accepts it. The set, address included, goes to the server:
   * call this when the user asks for live updates. After a drop the
   * connection comes back by itself, firing reconnecting and reconnect.
   */
  connect(): Promise<void> {
    if (this.#connecting !== null) {
      return this.#connecting.promise;
    }
    if (this.#wanted) {
      return Promise.resolve();
    }
    this.#wanted = true;
    let resolve: () => void = () => {};
    let reject: (err: GnotifError) => void = () => {};
    const promise = new Promise<void>((res, rej) => {
      resolve = res;
      reject = rej;
    });
    this.#connecting = { promise, resolve, reject };
    this.#open();
    return promise;
  }

  /** Closes the live connection and stops reconnecting. */
  disconnect(): void {
    if (!this.#wanted) {
      return;
    }
    this.#stop(new GnotifError("inactive", "Live connection closed."));
  }

  /**
   * Unsubscribes this browser first, then tells the server; a server that
   * no longer knows the subscription (404) is not an error.
   */
  async disable(): Promise<void> {
    const sub = await this.#subscription();
    await sub.unsubscribe();
    try {
      await this.#request("DELETE", "/v1/subscription", { endpoint: sub.endpoint });
    } catch (err) {
      if (!(err instanceof GnotifError && err.status === 404)) {
        throw err;
      }
    }
  }

  /**
   * Reports whether this browser holds a push subscription and still has
   * notification permission.
   */
  async enabled(): Promise<boolean> {
    if (!("Notification" in globalThis) || Notification.permission !== "granted") {
      return false;
    }
    return (await this.#find()) !== null;
  }

  /**
   * Ends the live channel: no socket, no timers, no retries, and every
   * waiting promise rejected with err.
   */
  #stop(err: GnotifError): void {
    const ws = this.#ws;
    this.#ws = null;
    this.#wanted = false;
    this.#reconnecting = false;
    this.#attempt = 0;
    this.#inFlight = false;
    this.#dirty = false;
    clearTimeout(this.#retry);
    clearTimeout(this.#pong);
    clearInterval(this.#ping);
    ws?.close(1000);
    this.#connecting?.reject(err);
    this.#connecting = null;
    for (const w of this.#waiters.splice(0)) {
      w.reject(err);
    }
  }

  /**
   * Ends the live channel with err, and fires error unless a connect() was
   * waiting, which err rejects instead.
   */
  #fail(err: GnotifError): void {
    const connecting = this.#connecting !== null;
    this.#stop(err);
    if (!connecting) {
      this.#emit("error", err);
    }
  }

  #open(): void {
    let ws: WebSocket;
    try {
      ws = new WebSocket(this.#server.replace(/^http/, "ws") + "/v1/stream");
    } catch (err) {
      // A browser refuses some URLs at once, such as ws: from an https:
      // page: the first connect fails, and a reconnect tries again later.
      if (this.#connecting !== null) {
        this.#stop(new GnotifError("network", err instanceof Error ? err.message : String(err)));
      } else {
        this.#onClose();
      }
      return;
    }
    this.#ws = ws;
    ws.addEventListener("open", () => {
      if (ws !== this.#ws) {
        return;
      }
      this.#send();
      this.#ping = setInterval(() => {
        ws.send(PING);
        clearTimeout(this.#pong);
        this.#pong = setTimeout(() => {
          // On a dead network a browser may report the close only much later.
          ws.close();
          if (ws === this.#ws) {
            this.#onClose();
          }
        }, PONG_TIMEOUT);
      }, PING_INTERVAL);
    });
    ws.addEventListener("message", (e) => {
      if (ws === this.#ws && typeof e.data === "string") {
        this.#onFrame(e.data);
      }
    });
    ws.addEventListener("close", (e) => {
      if (ws === this.#ws) {
        this.#onClose(e.code);
      }
    });
  }

  // #send sends the kept set when the socket is open and no frame awaits
  // its reply; otherwise the open, or that reply, sends it.
  #send(): void {
    const ws = this.#ws;
    if (ws === null || ws.readyState !== WebSocket.OPEN) {
      return;
    }
    if (this.#inFlight) {
      this.#dirty = true;
      return;
    }
    this.#inFlight = true;
    this.#dirty = false;
    this.#sentGen = this.#gen;
    ws.send(JSON.stringify({ type: "optins", optins: this.#items(false) }));
  }

  // #items returns the kept set as the server takes it, without the push key;
  // pushOnly leaves out the items with push: false.
  #items(pushOnly: boolean): { trigger: string; value: string }[] {
    return (this.#optins ?? [])
      .filter((o) => !pushOnly || o.push !== false)
      .map(({ trigger, value }) => ({ trigger, value }));
  }

  #onFrame(text: string): void {
    const frame = parseFrame(text);
    switch (frame?.type) {
      case "pong":
        clearTimeout(this.#pong);
        return;
      case "optins":
        this.#onReply(null);
        return;
      case "error":
        this.#onReply(frame.error ?? "gnotif server refused the opt-ins");
        return;
      case "notification": {
        const { type, ...fields } = frame;
        this.#deliver(browserNotification(fields, this.#scope(), "live"));
        return;
      }
      case undefined:
        // Unknown frame types, non-JSON text and known types with a mistyped
        // field all parse to no type; the client ignores them.
        return;
    }
  }

  // register() without a scope option scopes the worker to its URL's directory.
  #scope(): string {
    return new URL("./", new URL(this.#serviceWorker, location.href)).href;
  }

  /** @param data a message the service worker posted to this page */
  #onRelay(data: unknown): void {
    const fields = parseRelay(data);
    if (fields === undefined) {
      return;
    }
    const n = browserNotification(fields, this.#scope(), "push");
    if (this.#optins?.some((o) => o.trigger === n.trigger && o.value === n.value) === true) {
      this.#deliver(n);
    }
  }

  // #deliver emits n unless its id was among the last DELIVERED_MAX delivered.
  #deliver(n: GnotifNotification): void {
    if (this.#delivered.has(n.id)) {
      return;
    }
    this.#delivered.add(n.id);
    if (this.#delivered.size > DELIVERED_MAX) {
      for (const oldest of this.#delivered) {
        this.#delivered.delete(oldest);
        break;
      }
    }
    this.#emit("notification", n);
  }

  /** @param err the server's refusal, null when it accepted */
  #onReply(err: string | null): void {
    this.#inFlight = false;
    const settled = this.#waiters.filter((w) => w.gen <= this.#sentGen);
    this.#waiters = this.#waiters.filter((w) => w.gen > this.#sentGen);
    if (err === null) {
      this.#attempt = 0;
      const connecting = this.#connecting;
      this.#connecting = null;
      connecting?.resolve();
      if (this.#reconnecting) {
        this.#reconnecting = false;
        this.#emit("reconnect", undefined);
      }
      for (const w of settled) {
        w.resolve();
      }
    } else {
      const e = new GnotifError("server", err);
      if (this.#connecting !== null || this.#reconnecting) {
        // The set this connection opened with was refused, so it holds no
        // opt-ins: there is nothing to keep open.
        this.#fail(e);
        return;
      }
      for (const w of settled) {
        w.reject(e);
      }
      if (settled.length === 0) {
        this.#emit("error", e);
      }
    }
    if (this.#dirty) {
      this.#send();
    }
  }

  /** @param code the close code, when the socket reported one */
  #onClose(code?: number): void {
    this.#ws = null;
    this.#inFlight = false;
    clearInterval(this.#ping);
    clearTimeout(this.#pong);
    if (code === 1009 || code === 1003) {
      // The server refused a frame this client sent, and would refuse it
      // again on every reconnect.
      this.#fail(new GnotifError("server", `The server closed the live connection with code ${code}.`));
      return;
    }
    if (this.#connecting !== null) {
      this.#stop(new GnotifError("network", "The live connection closed before the server answered."));
      return;
    }
    if (!this.#reconnecting) {
      this.#reconnecting = true;
      this.#emit("reconnecting", undefined);
    }
    const delay = Math.min(BACKOFF_MAX, BACKOFF_MIN * 2 ** this.#attempt) * (0.5 + Math.random() / 2);
    this.#attempt++;
    this.#retry = setTimeout(() => this.#open(), delay);
  }

  #emit<K extends keyof GnotifEvents>(event: K, arg: GnotifEvents[K]): void {
    // A snapshot: a handler that another handler adds or removes takes
    // effect from the next event.
    for (const handler of Array.from(this.#handlers[event])) {
      try {
        handler(arg);
      } catch (err) {
        console.error(err);
      }
    }
  }

  // getRegistration resolves undefined when nothing is registered, where
  // serviceWorker.ready would never settle.
  async #find(): Promise<PushSubscription | null> {
    if (!("PushManager" in globalThis)) {
      return null;
    }
    const reg = await navigator.serviceWorker?.getRegistration();
    return (await reg?.pushManager.getSubscription()) ?? null;
  }

  async #subscription(): Promise<PushSubscription> {
    const sub = await this.#find();
    if (sub === null) {
      throw new GnotifError("inactive", "Notifications are not enabled in this browser.");
    }
    return sub;
  }

  /**
   * @param body sent as JSON
   * @returns the decoded JSON answer, null on 204
   */
  async #request(method: string, path: string, body?: unknown): Promise<unknown> {
    const init: RequestInit = { method };
    if (body !== undefined) {
      init.headers = { "Content-Type": "application/json" };
      init.body = JSON.stringify(body);
    }
    const res = await fetch(this.#server + path, init);
    if (!res.ok) {
      let message = `gnotif server answered ${res.status}`;
      try {
        message = parseErrorMessage(await res.json()) ?? message;
      } catch {
        // keep the status message
      }
      throw new GnotifError("server", message, res.status);
    }
    if (res.status === 204) {
      return null;
    }
    try {
      return await res.json();
    } catch {
      throw new GnotifError("server", `gnotif server answered ${res.status} with a body that is not JSON`);
    }
  }
}
