// gnotif browser client: subscribes this browser to a gnotif server's
// pushes and sets which triggers it hears about.

export class GnotifError extends Error {
  // code: "unsupported", "denied", "inactive" or "server"
  constructor(code, message) {
    super(message);
    this.name = "GnotifError";
    this.code = code;
  }
}

export function base64UrlToBytes(s) {
  const b64 = s.replaceAll("-", "+").replaceAll("_", "/").padEnd(Math.ceil(s.length / 4) * 4, "=");
  return Uint8Array.from(atob(b64), (c) => c.charCodeAt(0));
}

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

  constructor({ server, serviceWorker = "/sw.js" }) {
    this.#server = server.replace(/\/$/, "");
    this.#serviceWorker = serviceWorker;
  }

  async triggers() {
    return this.#request("GET", "/v1/triggers");
  }

  // enable must run from a user gesture: browsers refuse or hide a
  // permission prompt otherwise.
  async enable() {
    if (!("serviceWorker" in navigator) || !("PushManager" in globalThis) || !("Notification" in globalThis)) {
      throw new GnotifError("unsupported", "This browser has no Push API.");
    }
    if (await Notification.requestPermission() !== "granted") {
      throw new GnotifError("denied", "Notifications are not allowed for this site.");
    }
    await navigator.serviceWorker.register(`${this.#serviceWorker}?server=${encodeURIComponent(this.#server)}`);
    const reg = await navigator.serviceWorker.ready;
    const { publicKey } = await this.#request("GET", "/v1/vapid");
    const key = base64UrlToBytes(publicKey);
    let sub = await reg.pushManager.getSubscription();
    if (sub && !sameKey(sub.options.applicationServerKey, key)) {
      await sub.unsubscribe();
      sub = null;
    }
    sub ??= await reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key });
    const json = sub.toJSON();
    await this.#request("PUT", "/v1/subscription", json);
    return json;
  }

  async setOptins(optins) {
    const sub = await this.#subscription();
    await this.#request("PUT", "/v1/subscription/optins", { endpoint: sub.endpoint, optins });
  }

  async disable() {
    const sub = await this.#subscription();
    await this.#request("DELETE", "/v1/subscription", { endpoint: sub.endpoint });
    await sub.unsubscribe();
  }

  async #subscription() {
    const reg = await navigator.serviceWorker?.ready;
    const sub = await reg?.pushManager.getSubscription();
    if (!sub) {
      throw new GnotifError("inactive", "Notifications are not enabled in this browser.");
    }
    return sub;
  }

  async #request(method, path, body) {
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
      throw new GnotifError("server", message);
    }
    return res.status === 204 ? null : res.json();
  }
}
