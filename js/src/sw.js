// gnotif service worker. A dapp serves a copy of this file from its own
// origin and registers it as sw.js?server=<gnotif server URL>, plus an
// optional &icon=<image URL> that every notification shows.

const sw = /** @type {ServiceWorkerGlobalScope} */ (/** @type {unknown} */ (self));

/**
 * @param {string} text push payload
 * @returns {{ title: string, body: string, link: string }}
 */
function parsePayload(text) {
  try {
    const p = JSON.parse(text);
    if (p && typeof p.title === "string" && p.title !== "" && typeof p.body === "string" && typeof p.link === "string") {
      return { title: p.title, body: p.body, link: p.link };
    }
  } catch {
    // fall through to the generic notification
  }
  return { title: "New activity", body: "", link: "./" };
}

// safeLink resolves link against the worker's scope and refuses anything
// that lands on another origin, such as "//host" or "/\host": those open
// the scope instead.
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
    // an unparsable link opens the scope
  }
  return scope;
}

// Safari revokes a subscription whose pushes show nothing, so every push
// shows a notification, a generic one when the payload is unusable. Pushes
// with the same link replace each other; renotify makes the replacement
// alert again instead of updating silently.
sw.addEventListener("push", (event) => {
  const p = parsePayload(event.data ? event.data.text() : "");
  /** @type {NotificationOptions & { renotify: boolean }} */
  const options = { body: p.body, tag: p.link, renotify: true, data: { link: p.link } };
  const icon = new URL(sw.location.href).searchParams.get("icon");
  if (icon !== null) {
    options.icon = icon;
  }
  event.waitUntil(sw.registration.showNotification(p.title, options));
});

sw.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url = safeLink(event.notification.data?.link ?? "./", sw.registration.scope);
  event.waitUntil((async () => {
    const windows = await sw.clients.matchAll({ type: "window", includeUncontrolled: true });
    for (const client of windows) {
      if (client.url.startsWith(sw.registration.scope)) {
        // Focus first: a browser may refuse a focus that comes after the
        // navigation finishes.
        await client.focus();
        try {
          await client.navigate(url);
        } catch {
          // an uncontrolled tab refuses navigate; focusing it is enough
        }
        return;
      }
    }
    return sw.clients.openWindow(url);
  })());
});

// A rotated subscription is registered in place of the old endpoint, so the
// server keeps its opt-ins. Both subscriptions may be null.
sw.addEventListener("pushsubscriptionchange", (event) => {
  const server = new URL(sw.location.href).searchParams.get("server");
  event.waitUntil((async () => {
    if (server === null) {
      throw new Error("sw.js was registered without a ?server= URL");
    }
    const sub = event.newSubscription ?? await subscribeAgain(server);
    const res = await fetch(server + "/v1/subscription", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ ...sub.toJSON(), oldEndpoint: event.oldSubscription?.endpoint }),
    });
    if (!res.ok) {
      throw new Error(`gnotif server answered ${res.status}`);
    }
  })());
});

/**
 * @param {string} server
 * @returns {Promise<PushSubscription>}
 */
async function subscribeAgain(server) {
  const res = await fetch(server + "/v1/vapid");
  if (!res.ok) {
    throw new Error(`gnotif server answered ${res.status}`);
  }
  const { publicKey } = await res.json();
  // applicationServerKey takes the base64url key as served.
  return sw.registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: publicKey });
}
