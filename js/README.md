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
import { Gnotif } from "gnotif";

const gnotif = new Gnotif({ server: "https://gnotif.example", serviceWorker: "/sw.js" });
```

`serviceWorker` is the URL of the copy on the dapp's origin and defaults
to `/sw.js`.

- `triggers()` returns the triggers the server offers.
- `enable()` asks for notification permission, subscribes the browser and
  registers it with the server. Call it from a user gesture such as a
  click, or the browser may hide the permission prompt. It throws a
  `GnotifError` with code `unsupported`, `denied` or `server`.
- `setOptins(optins)` replaces the browser's opt-ins, a list of
  `{ trigger, value }`. It throws `GnotifError` with code `inactive` when
  the browser is not subscribed.
- `enabled()` returns `true` when the browser holds a push subscription
  and notification permission is granted. It returns `false` after the
  user revokes permission, so offer `enable()` again.
- `disable()` unsubscribes the browser and removes the subscription from
  the server. A server that no longer knows the subscription is not an
  error.

Notification clicks focus an open tab of the dapp and navigate it to the
notification's link, or open a new tab. Links to other origins open the
dapp's root.

Types ship with the package as `.d.ts` files.

## License

Apache-2.0
