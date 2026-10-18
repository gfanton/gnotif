// gnotif service worker. It reads the server and the icon from its own URL:
// sw.js?server=<gnotif server URL>&icon=<image URL>, the icon optional.

import { safeLink } from "../link.ts";
import { parseClickLink, parsePayload, parseVapidKey, relayable } from "../push.ts";

declare const self: ServiceWorkerGlobalScope;

function inScope(client: WindowClient): boolean {
  return client.url.startsWith(self.registration.scope);
}

// relayToWindows posts a full payload to each window in scope.
async function relayToWindows(text: string): Promise<void> {
  const notification = relayable(text);
  if (notification === undefined) {
    return;
  }
  const windows = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
  for (const client of windows) {
    if (inScope(client)) {
      try {
        client.postMessage({ type: "gnotif.notification", notification });
      } catch {
        // a window that has gone away refuses the message
      }
    }
  }
}

// Safari revokes a subscription whose pushes show nothing, so every push
// shows a notification, a generic one when the payload is unusable. Pushes
// with the same link replace each other; renotify makes the replacement
// alert again instead of updating silently.
self.addEventListener("push", (event) => {
  const text = event.data !== null ? event.data.text() : "";
  const p = parsePayload(text);
  const options: NotificationOptions & { renotify: boolean } = {
    body: p.body,
    tag: p.link,
    renotify: true,
    data: { link: p.link },
  };
  const icon = new URL(self.location.href).searchParams.get("icon");
  if (icon !== null) {
    options.icon = icon;
  }
  // The relay is best-effort: its failure must not fail the push.
  event.waitUntil(
    Promise.all([self.registration.showNotification(p.title, options), relayToWindows(text).catch(() => {})]),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const url = safeLink(parseClickLink(event.notification.data), self.registration.scope);
  event.waitUntil(
    (async () => {
      const windows = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
      for (const client of windows) {
        if (inScope(client)) {
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
      await self.clients.openWindow(url);
    })(),
  );
});

// A rotated subscription is registered in place of the old endpoint, so the
// server keeps its opt-ins. Both subscriptions may be null.
self.addEventListener("pushsubscriptionchange", (event) => {
  const server = new URL(self.location.href).searchParams.get("server");
  event.waitUntil(
    (async () => {
      if (server === null) {
        throw new Error("sw.js was registered without a ?server= URL");
      }
      const sub = event.newSubscription ?? (await subscribeAgain(server));
      const res = await fetch(server + "/v1/subscription", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ...sub.toJSON(), oldEndpoint: event.oldSubscription?.endpoint }),
      });
      if (!res.ok) {
        throw new Error(`gnotif server answered ${res.status}`);
      }
    })(),
  );
});

async function subscribeAgain(server: string): Promise<PushSubscription> {
  const res = await fetch(server + "/v1/vapid");
  if (!res.ok) {
    throw new Error(`gnotif server answered ${res.status}`);
  }
  const publicKey = parseVapidKey(await res.json());
  if (publicKey === undefined) {
    throw new Error("gnotif server answered no VAPID key");
  }
  // applicationServerKey takes the base64url key as served.
  return self.registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: publicKey });
}
