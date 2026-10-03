// gnotif browser client: subscribes this browser to a gnotif server's
// pushes and sets which triggers it hears about.

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

export class GnotifError extends Error {
  /**
   * @param {"unsupported" | "denied" | "inactive" | "server"} code
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

export class Gnotif {
  #server;
  #serviceWorker;

  /**
   * @param {object} options
   * @param {string} options.server base URL of the gnotif server
   * @param {string} [options.serviceWorker] URL of the dapp's copy of sw.js
   */
  constructor({ server, serviceWorker = "/sw.js" }) {
    this.#server = server.replace(/\/$/, "");
    this.#serviceWorker = serviceWorker;
  }

  /** @returns {Promise<Trigger[]>} the triggers the server offers */
  async triggers() {
    return /** @type {Trigger[]} */ (await this.#request("GET", "/v1/triggers"));
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
    await navigator.serviceWorker.register(`${this.#serviceWorker}?server=${encodeURIComponent(this.#server)}`);
    const reg = await navigator.serviceWorker.ready;
    const { publicKey } = /** @type {{ publicKey: string }} */ (await this.#request("GET", "/v1/vapid"));
    const key = base64UrlToBytes(publicKey);
    let sub = await reg.pushManager.getSubscription();
    if (sub && !sameKey(sub.options.applicationServerKey, key)) {
      await sub.unsubscribe();
      sub = null;
    }
    sub ??= await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: new Uint8Array(key) });
    const json = sub.toJSON();
    await this.#request("PUT", "/v1/subscription", json);
    return json;
  }

  /**
   * @param {{ trigger: string, value: string }[]} optins replaces this
   *   browser's opt-ins
   * @returns {Promise<void>}
   */
  async setOptins(optins) {
    const sub = await this.#subscription();
    await this.#request("PUT", "/v1/subscription/optins", { endpoint: sub.endpoint, optins });
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
