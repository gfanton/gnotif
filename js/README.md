# gnotif

Browser client for a gnotif server, which sends Web Push notifications for
gno.land realm events.

## Install

```sh
npm install gnotif
```

## Serve the service worker

A service worker only controls the origin that serves it, so the dapp
serves its own copy of `sw.js`:

```sh
cp node_modules/gnotif/src/sw.js public/sw.js
```

The package exports the file as `gnotif/sw.js` for build tools that copy
assets by module path. Copy it again after each upgrade.

## Use

```js
import { Gnotif, GnotifError } from "gnotif";

const gnotif = new Gnotif({ network: "onyx", serviceWorker: "/sw.js", icon: "/icon.png" });
```

`network` names the gno.land network and its hosted server: `"onyx"`,
served by `https://gnotif.xyz/onyx`, is the default and the only one for
now. `server` points the client at another gnotif server, such as your
own gnotifd; `network` is then only a label. `gnotif.network` and
`gnotif.server` read both back. `serviceWorker` is the URL
of the copy on the dapp's origin and defaults to `/sw.js`. A dapp served
under a path, such as `/app/`, serves its copy from that path and passes
`/app/sw.js`.

A dapp reaches the browser in two ways, each optional: push, which
shows OS notifications even with the tab closed, and live updates,
which reach the open page. Both use the one opt-in set from
`setOptins`, and each sends the server only this:

| Channel | Sent to the server |
|---|---|
| push | the push subscription's endpoint and keys, and the opt-ins |
| live updates | the opt-ins |

An opt-in's value is often the user's address, so turn either channel
on when the user asks for it.

`icon` is the URL of an image every notification shows, such as the
dapp's logo; a relative URL resolves against the page. Use a PNG: a
browser may not show an SVG in a notification. Safari on macOS ignores
`icon` and shows its own. A browser subscribed before the dapp set
`icon` gets it only after `enable()` runs again.

### `triggers(target)`

Resolves the verified triggers of the realm at `target`, a realm path
such as `gno.land/r/<you>/notify`, in id order, or `[]` when the server
knows none. [Triggers](../docs/triggers.md#fields) describes their
fields.

```js
const [message] = await gnotif.triggers("gno.land/r/<you>/notify");
```

A trigger's id can change, so call it on every page load rather than
keeping an id. It rejects with a `TypeError`, before any request, when
`target` is not a non-empty string, and throws a `GnotifError` with code
`server` when the server answers an error.

### `enable()`

Asks for notification permission, subscribes the browser, and registers
it with the server, with the opt-ins `setOptins` kept if it was called.
It resolves the subscription as `PushSubscriptionJSON`. Call it from a
user gesture such as a click, or the browser may refuse or hide the
permission prompt.

It throws a `GnotifError` with code:

- `unsupported` when the browser has no service worker, Push or
  Notification API.
- `denied` when permission is not granted. Once the user has blocked
  notifications for the site, `enable()` throws `denied` at once, without
  a prompt, until the browser's site settings lift the block.
- `server` when the server answers an error, to the request for its
  VAPID key (`GET /v1/vapid`) or to the subscription.

### `setOptins(optins)`

Keeps `optins`, a list of `{ trigger, value }` where `value` is `""` for
a trigger without a param, as the set both channels use. It sends the
set to the live connection when one is open, and to the server for push
when the browser holds a push subscription, and resolves once each has
accepted it. With neither, it only keeps the set, for a later
`enable()` or `connect()`. It throws a `GnotifError` with code `server`
when the server refuses the set: for push, status 400 for an opt-in
that breaks a rule and 404 when the server does not know the
subscription, which `enable()` registers again.

The server cannot read opt-ins back, so keep the set you last sent.

### `enabled()`

Resolves `true` when the browser holds a push subscription and
notification permission is granted. It resolves `false` after the user
revokes permission, even though the subscription remains. Offer
`enable()` again then: it prompts when permission went back to
`default`, and throws `denied` when the user blocked notifications.

### `disable()`

Unsubscribes the browser first, then asks the server to delete the
subscription. A 404, which means the server does not know the
subscription, is not an error. It throws a `GnotifError` with code
`inactive` when the browser holds no push subscription, and `server` on
any other server error.

## Live updates

An open page can receive its notifications as they happen, with no
permission prompt and no service worker:

```js
const off = gnotif.on("notification", (n) => show(n));
gnotif.on("reconnect", () => refreshFromChain());

await gnotif.setOptins(optins);
await gnotif.connect();
```

### `on(event, handler)`

Calls `handler` on every event of that name, and returns a function
that removes it. A handler that throws is reported with
`console.error`, and the other handlers still run.

| Event | The handler receives | When |
|---|---|---|
| `notification` | `trigger`, `value`, `title`, `body`, `link`, `height`, `tx`, `time` and `attrs` | a live notification arrives |
| `disconnect` | nothing | the connection dropped and is retrying |
| `reconnect` | nothing | it is back, with the set sent again |
| `error` | a `GnotifError` | a live failure no promise waits on, such as the set refused after a reconnect |

`link` arrives resolved on the page's origin. A link that would land on
another origin arrives as the service worker's scope, as for a push: the
path `sw.js` is served from, `/` for a dapp at the root of its origin.
`attrs` lists the event's attributes as `{ key, value }`, `height` and
`tx` name the transaction that emitted the event, and `time` is the
block's time, in UTC, as RFC 3339. A frame without `time` reaches no
handler, so this client needs a server that sends it.

A notification is a hint: read the state the page shows from the chain.
Nothing is replayed after a drop, so read it again on `reconnect`.

### `connect()`

Opens the live connection with the set `setOptins` kept, and resolves
once the server accepts it. It sends that set, address included, to the
server, so call it when the user asks for live updates. It throws a
`GnotifError` with code `server` when the server refuses the set or the
connection cannot open, and does not retry then. After a drop, the
connection comes back by itself.

### `disconnect()`

Closes the live connection and stops reconnecting. A `connect()` still
waiting rejects with code `inactive`.

## Errors

`GnotifError` has three fields:

- `code`: `"unsupported"`, `"denied"`, `"inactive"` or `"server"`;
- `message`: for `server`, the server's error text when it sent one,
  otherwise `gnotif server answered <status>`;
- `status`: the HTTP status of a `server` error, `undefined` for the
  other codes.

A network failure, or a failure inside the browser's Push API, rejects
with the browser's own error rather than a `GnotifError`.

## Notifications

`sw.js` shows every push with its title and body, and the `icon` the
dapp passed, if any. A notification replaces an earlier one with the
same link. A push without a usable payload still shows "New activity",
because Safari revokes a subscription whose pushes show nothing.

A click opens the notification's link on the dapp's origin. A link that
lands on another origin opens the service worker's scope instead: the
path `sw.js` is served from, `/` for a dapp at the root of its origin.
"New activity" opens the scope too. A click focuses an open tab of the
dapp, or opens a new one. A tab the service worker does not control,
such as the one where `enable()` first ran, is focused but not moved to
the link.

When the browser renews its push subscription, `sw.js` sends the new one
to the server in place of the old, so the opt-ins carry over.

## Types

Types ship with the package as `.d.ts` files, including the `Trigger`
type that `triggers()` resolves, `Optin`, and the `LiveNotification`
that `notification` handlers receive.

## License

Apache-2.0
