// gnotif browser client: subscribes this browser to a gnotif server's
// pushes, holds an open page's live connection, and sets which triggers
// both hear about.

/**
 * A trigger as GET /v1/triggers returns it.
 * @typedef {object} Trigger
 * @property {string} id
 * @property {string} target realm path the event comes from
 * @property {string} event
 * @property {string} filter comma-separated key=value pairs, "" for none
 * @property {string} param name of the parameter a user opts into, "" for none
 * @property {string} title
 * @property {string} body
 * @property {string} link
 * @property {string} declarer address that declared the trigger
 * @property {boolean} verified
 */

/**
 * One trigger a browser asks to hear about, for one param value.
 * @typedef {object} Optin
 * @property {string} trigger trigger id
 * @property {string} value the param value, "" for a trigger without a param
 * @property {boolean} [push] false keeps the item off push: the page hears
 *   it live only. Push stays on when absent.
 */

/**
 * A notification as either channel delivers it.
 * @typedef {object} GnotifNotification
 * @property {string} id identifies the match; both channels carry the same one
 * @property {"live" | "push"} source the channel that delivered it first
 * @property {string} trigger trigger id
 * @property {string} value the opted-in param value, "" for none
 * @property {string} title
 * @property {string} body
 * @property {string} link absolute URL on the page's origin
 * @property {number} height block height of the event
 * @property {string} tx hash of the transaction that emitted the event
 * @property {string} time the block's time, in UTC, as RFC 3339
 * @property {{ key: string, value: string }[]} attrs the event's attributes, in order
 */

/**
 * What each event's handler receives.
 * @typedef {object} GnotifEvents
 * @property {GnotifNotification} notification a notification arrived, live or by push
 * @property {undefined} reconnecting the live connection dropped and is retrying
 * @property {undefined} reconnect it is back; notifications in between were missed
 * @property {GnotifError} error a live failure no promise waits on
 */

const PING = '{"type":"ping"}';
const PING_INTERVAL = 30_000;
const PONG_TIMEOUT = 10_000;
const BACKOFF_MIN = 1_000;
const BACKOFF_MAX = 30_000;
const DELIVERED_MAX = 256;

/**
 * @param {unknown} v
 * @returns {v is Record<string, unknown>}
 */
function isRecord(v) {
  return typeof v === "object" && v !== null;
}

/**
 * @param {unknown} v
 * @returns {v is { key: string, value: string }}
 */
function isAttr(v) {
  return isRecord(v) && typeof v.key === "string" && typeof v.value === "string";
}

// safeLink resolves link against the service worker's scope, and gives the
// scope for a link that lands on another origin, such as "//host" or "/\host".
/**
 * @param {string} link
 * @param {string} scope
 * @returns {string}
 */
function safeLink(link, scope) {
  try {
    const url = new URL(link, scope);
    if (url.origin === new URL(scope).origin) {
      return url.href;
    }
  } catch {
    // an unparsable link gives the scope
  }
  return scope;
}

/**
 * @param {Record<string, unknown>} f a decoded notification frame or relayed push
 * @param {string} scope the service worker's scope
 * @param {"live" | "push"} source
 * @returns {GnotifNotification | null} null when a field is missing or mistyped
 */
function browserNotification(f, scope, source) {
  const { id, trigger, value, title, body, link, height, tx, time, attrs } = f;
  if (typeof id !== "string" || typeof trigger !== "string" || typeof value !== "string" || typeof title !== "string" ||
    typeof body !== "string" || typeof link !== "string" || typeof height !== "number" ||
    typeof tx !== "string" || typeof time !== "string" || !Array.isArray(attrs) || !attrs.every(isAttr)) {
    return null;
  }
  return { id, source, trigger, value, title, body, link: safeLink(link, scope), height, tx, time, attrs };
}

export class GnotifError extends Error {
  /**
   * @param {"unsupported" | "denied" | "inactive" | "server" | "network"} code
   * @param {string} message
   * @param {number} [status] HTTP status of a "server" error
   */
  constructor(code, message, status) {
    super(message);
    this.status = status;
    this.name = "GnotifError";
    this.code = code;
  }
}

/**
 * @param {string} s unpadded base64url text
 * @returns {Uint8Array}
 */
export function base64UrlToBytes(s) {
  const b64 = s.replaceAll("-", "+").replaceAll("_", "/").padEnd(Math.ceil(s.length / 4) * 4, "=");
  return Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
}

/**
 * @param {ArrayBuffer | null | undefined} a
 * @param {Uint8Array} b
 * @returns {boolean}
 */
export function sameKey(a, b) {
  if (!a) {
    return false;
  }
  const bytes = new Uint8Array(a);
  return bytes.length === b.length && bytes.every((v, i) => v === b[i]);
}

/** The hosted gnotif server of each gno.land network. */
const servers = new Map([["onyx", "https://gnotif.xyz/onyx"]]);

export class Gnotif {
  #network;
  #server;
  #serviceWorker;
  #icon;
  /** @type {Optin[] | null} the set every channel uses, null until setOptins */
  #optins = null;
  /** @type {{ [K in keyof GnotifEvents]: Set<(arg: GnotifEvents[K]) => void> }} */
  #handlers = { notification: new Set(), reconnecting: new Set(), reconnect: new Set(), error: new Set() };
  /** @type {Set<string>} the last DELIVERED_MAX delivered ids, oldest first */
  #delivered = new Set();

  // ---- Live connection state
  /** @type {WebSocket | null} */
  #ws = null;
  #wanted = false; // connect() was called and disconnect() was not
  #reconnecting = false; // dropped after its set was accepted, and retrying
  #attempt = 0; // retries since the last accepted set
  /** @type {{ promise: Promise<void>, resolve: () => void, reject: (err: GnotifError) => void } | null} */
  #connecting = null;
  #gen = 0; // counts setOptins calls
  #sentGen = 0; // the generation of the optins frame last sent
  #inFlight = false; // an optins frame awaits its reply
  #dirty = false; // the kept set changed while a frame was in flight
  /** @type {{ gen: number, resolve: () => void, reject: (err: GnotifError) => void }[]} */
  #waiters = [];
  /** @type {ReturnType<typeof setTimeout> | undefined} */
  #retry;
  /** @type {ReturnType<typeof setTimeout> | undefined} */
  #pong;
  /** @type {ReturnType<typeof setInterval> | undefined} */
  #ping;

  /**
   * @param {object} [options]
   * @param {string} [options.network] gno.land network whose hosted server to
   *   use, "onyx" by default; only a label when server is set
   * @param {string} [options.server] base URL of another gnotif server, such
   *   as your own gnotifd
   * @param {string} [options.serviceWorker] URL of the dapp's copy of sw.js
   * @param {string} [options.icon] URL of the image every notification
   *   shows, such as a PNG of the dapp's logo, relative to the page
   */
  constructor({ network, server, serviceWorker = "/sw.js", icon } = {}) {
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

  /** @returns {string | undefined} the network named at construction */
  get network() {
    return this.#network;
  }

  /** @returns {string} base URL of the gnotif server this client calls */
  get server() {
    return this.#server;
  }

  /**
   * @param {string} target realm path whose verified triggers to list
   * @returns {Promise<Trigger[]>} the triggers that realm offers
   */
  async triggers(target) {
    if (typeof target !== "string" || target === "") {
      throw new TypeError("target must be a non-empty realm path");
    }
    return /** @type {Trigger[]} */ (
      await this.#request("GET", `/v1/triggers?target=${encodeURIComponent(target)}`)
    );
  }

  /**
   * Subscribes this browser and registers it with the server. Must run from
   * a user gesture: browsers refuse or hide a permission prompt otherwise.
   * @returns {Promise<PushSubscriptionJSON>}
   */
  async enable() {
    if (!("serviceWorker" in navigator) || !("PushManager" in globalThis) || !("Notification" in globalThis)) {
      throw new GnotifError("unsupported", "This browser has no Push API.");
    }
    if (await Notification.requestPermission() !== "granted") {
      throw new GnotifError("denied", "Notifications are not allowed for this site.");
    }
    let url = `${this.#serviceWorker}?server=${encodeURIComponent(this.#server)}`;
    if (this.#icon !== undefined) {
      url += `&icon=${encodeURIComponent(new URL(this.#icon, document.baseURI).href)}`;
    }
    await navigator.serviceWorker.register(url);
    const reg = await navigator.serviceWorker.ready;
    const { publicKey } = /** @type {{ publicKey: string }} */ (await this.#request("GET", "/v1/vapid"));
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
   * @param {Optin[]} optins replaces the previous set
   * @returns {Promise<void>}
   */
  async setOptins(optins) {
    this.#optins = optins;
    const gen = ++this.#gen;
    /** @type {Promise<unknown>[]} */
    const pending = [];
    if (this.#wanted && this.#ws?.readyState === WebSocket.OPEN) {
      pending.push(new Promise((resolve, reject) => {
        this.#waiters.push({ gen, resolve: () => resolve(undefined), reject });
      }));
      this.#send();
    }
    pending.push(this.#find().then((sub) =>
      sub !== null ? this.#request("PUT", "/v1/subscription/optins", { endpoint: sub.endpoint, optins: this.#items(true) }) : null));
    await Promise.all(pending);
  }

  /**
   * Calls handler on every event of that name until the returned function
   * runs.
   * @template {keyof GnotifEvents} K
   * @param {K} event
   * @param {(arg: GnotifEvents[K]) => void} handler
   * @returns {() => void} removes the handler
   */
  on(event, handler) {
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
   * @returns {Promise<void>}
   */
  connect() {
    if (this.#connecting !== null) {
      return this.#connecting.promise;
    }
    if (this.#wanted) {
      return Promise.resolve();
    }
    this.#wanted = true;
    /** @type {() => void} */
    let resolve = () => {};
    /** @type {(err: GnotifError) => void} */
    let reject = () => {};
    /** @type {Promise<void>} */
    const promise = new Promise((res, rej) => {
      resolve = res;
      reject = rej;
    });
    this.#connecting = { promise, resolve, reject };
    this.#open();
    return promise;
  }

  /** Closes the live connection and stops reconnecting. */
  disconnect() {
    if (!this.#wanted) {
      return;
    }
    this.#stop(new GnotifError("inactive", "Live connection closed."));
  }

  /**
   * Unsubscribes this browser first, then tells the server; a server that
   * no longer knows the subscription (404) is not an error.
   * @returns {Promise<void>}
   */
  async disable() {
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
   * @returns {Promise<boolean>}
   */
  async enabled() {
    if (!("Notification" in globalThis) || Notification.permission !== "granted") {
      return false;
    }
    return (await this.#find()) != null;
  }

  /**
   * Ends the live channel: no socket, no timers, no retries, and every
   * waiting promise rejected with err.
   * @param {GnotifError} err
   */
  #stop(err) {
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
   * @param {GnotifError} err
   */
  #fail(err) {
    const connecting = this.#connecting !== null;
    this.#stop(err);
    if (!connecting) {
      this.#emit("error", err);
    }
  }

  #open() {
    /** @type {WebSocket} */
    let ws;
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
  #send() {
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

  // #items returns the kept set as the server takes it, without the push key.
  /**
   * @param {boolean} pushOnly leave out the items with push: false
   * @returns {{ trigger: string, value: string }[]}
   */
  #items(pushOnly) {
    return (this.#optins ?? [])
      .filter((o) => !pushOnly || o.push !== false)
      .map(({ trigger, value }) => ({ trigger, value }));
  }

  /** @param {string} text */
  #onFrame(text) {
    /** @type {unknown} */
    let frame;
    try {
      frame = JSON.parse(text);
    } catch {
      return;
    }
    if (!isRecord(frame)) {
      return;
    }
    switch (frame.type) {
      case "pong":
        clearTimeout(this.#pong);
        return;
      case "optins":
        this.#onReply(null);
        return;
      case "error":
        this.#onReply(typeof frame.error === "string" ? frame.error : "gnotif server refused the opt-ins");
        return;
      case "notification": {
        const n = browserNotification(frame, this.#scope(), "live");
        if (n !== null) {
          this.#deliver(n);
        }
        return;
      }
      default:
        // A server may add frame types; this client ignores the ones it
        // does not know.
        return;
    }
  }

  // register() without a scope option scopes the worker to its URL's directory.
  /** @returns {string} the service worker's scope */
  #scope() {
    return new URL("./", new URL(this.#serviceWorker, location.href)).href;
  }

  /** @param {unknown} data a message the service worker posted to this page */
  #onRelay(data) {
    if (!isRecord(data) || data.type !== "gnotif.notification" || !isRecord(data.notification)) {
      return;
    }
    const n = browserNotification(data.notification, this.#scope(), "push");
    if (n !== null && this.#optins?.some((o) => o.trigger === n.trigger && o.value === n.value)) {
      this.#deliver(n);
    }
  }

  // #deliver emits n unless its id was among the last DELIVERED_MAX delivered.
  /** @param {GnotifNotification} n */
  #deliver(n) {
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

  /** @param {string | null} err the server's refusal, null when it accepted */
  #onReply(err) {
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

  /** @param {number} [code] the close code, when the socket reported one */
  #onClose(code) {
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

  /**
   * @template {keyof GnotifEvents} K
   * @param {K} event
   * @param {GnotifEvents[K]} arg
   */
  #emit(event, arg) {
    for (const handler of [...this.#handlers[event]]) {
      try {
        handler(arg);
      } catch (err) {
        console.error(err);
      }
    }
  }

  // getRegistration resolves undefined when nothing is registered, where
  // serviceWorker.ready would never settle.
  /** @returns {Promise<PushSubscription | null>} */
  async #find() {
    if (!("PushManager" in globalThis)) {
      return null;
    }
    const reg = await navigator.serviceWorker?.getRegistration();
    return (await reg?.pushManager.getSubscription()) ?? null;
  }

  /** @returns {Promise<PushSubscription>} */
  async #subscription() {
    const sub = await this.#find();
    if (!sub) {
      throw new GnotifError("inactive", "Notifications are not enabled in this browser.");
    }
    return sub;
  }

  /**
   * @param {string} method
   * @param {string} path
   * @param {unknown} [body] sent as JSON
   * @returns {Promise<unknown>} the decoded JSON answer, null on 204
   */
  async #request(method, path, body) {
    /** @type {RequestInit} */
    const init = { method };
    if (body !== undefined) {
      init.headers = { "Content-Type": "application/json" };
      init.body = JSON.stringify(body);
    }
    const res = await fetch(this.#server + path, init);
    if (!res.ok) {
      let message = `gnotif server answered ${res.status}`;
      try {
        message = (await res.json()).error ?? message;
      } catch {
        // keep the status message
      }
      throw new GnotifError("server", message, res.status);
    }
    return res.status === 204 ? null : res.json();
  }
}
