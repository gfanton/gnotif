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

const gnotif = new Gnotif({ server: "https://gnotif.xyz", serviceWorker: "/sw.js" });
```

`server` is the base URL of the gnotif server. `serviceWorker` is the URL
of the copy on the dapp's origin and defaults to `/sw.js`. A dapp served
under a path, such as `/app/`, serves its copy from that path and passes
`/app/sw.js`.

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
it with the server. It resolves the subscription as
`PushSubscriptionJSON`. Call it from a user gesture such as a click, or
the browser may refuse or hide the permission prompt.

It throws a `GnotifError` with code:

- `unsupported` when the browser has no service worker, Push or
  Notification API.
- `denied` when permission is not granted. Once the user has blocked
  notifications for the site, `enable()` throws `denied` at once, without
  a prompt, until the browser's site settings lift the block.
- `server` when the server answers an error, to the request for its
  VAPID key (`GET /v1/vapid`) or to the subscription.

### `setOptins(optins)`

Replaces the browser's opt-ins with `optins`, a list of
`{ trigger, value }`, where `value` is `""` for a trigger without a
param. It throws a `GnotifError` with code `inactive` when the browser
holds no push subscription, and `server` when the server refuses the
set: status 400 for an opt-in that breaks a rule, 404 when the server
does not know the subscription. Call `enable()` to register it again.

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

`sw.js` shows every push with its title and body. A notification
replaces an earlier one with the same link. A push without a usable
payload still shows "New activity", because Safari revokes a
subscription whose pushes show nothing.

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
type that `triggers()` resolves.

## License

Apache-2.0
