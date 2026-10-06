# gnotif

gnotif sends browser notifications when something happens on a gno.land chain, even when the page that asked for them is closed. A dapp's realm emits events, and a trigger declared in the gnotif registry realm says which event notifies whom. gnotifd reads the chain through tx-indexer and sends a Web Push for each match. The dapp's page subscribes the browser with the `gnotif` npm package.

```
+-- gno.land chain --------------------------------+
|  dapp realm ----Declare----> gnotif registry     |
+------+----------------------------+--------------+
       | realm events               | TriggerDeclared, TriggerRemoved
       v                            v
+--------------------------------------------------+
|                    tx-indexer                    |
+------------------------+-------------------------+
                         | polled over GraphQL
                         v
dapp page --/v1-->    gnotifd --Web Push--> push service
    ^                                            |
    | opens the link on click                    |
    +---- sw.js on the dapp's origin <-----------+
```

The page calls gnotifd only to subscribe and to choose its triggers. The notification travels through the browser's push service and is shown by `sw.js`, the service worker the dapp serves from its own origin, so it arrives even when no tab of the dapp is open.

## Add notifications to a dapp

1. **Emit an event** from your realm. This realm sends a message to an address:

   ```go
   // Notify sends msg to the browsers that opted in with the address to.
   func Notify(cur realm, to address, msg string) {
   	chain.Emit("Message", "to", to.String(), "msg", msg)
   }
   ```

2. **Declare a trigger** in the registry that gnotif.xyz reads, from the realm's `init` function, so the deploy declares it. Declared by the realm itself, the trigger is verified:

   ```go
   import "gno.land/r/<namespace>/gnotif/v0"

   func init(cur realm) {
   	gnotif.Declare(cross(cur), cur.PkgPath(), "Message", "", "to", "New message", "{msg}", "/")
   }
   ```

   This trigger notifies the browsers that opted in with the address in the event's `to` attribute. A click opens `/` on the dapp's own origin.

3. **Add the client** to the dapp's page, and serve a copy of its service worker from the dapp's origin:

   ```sh
   npm install gnotif
   cp node_modules/gnotif/src/sw.js public/sw.js
   ```

   ```js
   import { Gnotif } from "gnotif";

   const gnotif = new Gnotif({ server: "https://gnotif.xyz" });
   const [message] = await gnotif.triggers("gno.land/r/<you>/notify");

   button.addEventListener("click", async () => {
     await gnotif.enable();
     await gnotif.setOptins([{ trigger: message.id, value: address }]);
   });
   ```

   `triggers()` lists your realm's verified triggers, here the one from step 2, where `<you>` is the address that deployed the realm. `enable()` asks for permission, so call it from a click.

[Getting started](docs/getting-started.md) covers each step and tries the realm on onyx.

## Run gnotifd

gnotif.xyz is a public gnotifd that any dapp can use. To run your own, follow [Running gnotifd](docs/running-gnotifd.md): it covers installing gnotifd, making its VAPID keys and starting it.

## Documentation

- [Getting started](docs/getting-started.md): from realm to page, tried on onyx.
- [Triggers](docs/triggers.md): the fields, templates and matching rules, the verified mark and the limit per realm.
- [How it works](docs/how-it-works.md): the watch window, storage, delivery outcomes and the rules for gnotif servers.
- [Browser client](js/README.md): the `gnotif` npm package.
- [HTTP API](docs/http-api.md): the `/v1` endpoints, errors and CORS.
- [Running gnotifd](docs/running-gnotifd.md): flags, VAPID keys, the push service allowlist, the indexer and the container image.
- [Deploying the registry](docs/deploying-realms.md): deploy copies, linting and the registry's deploy on onyx.
- [The demo](demo/README.md): pingpong, run on a local chain or deployed to onyx.

## Develop

`make test` runs the realm tests, the Go tests and the browser client's tests. It needs Go, Node.js and the gno toolchain of the release onyx runs, which CI pins as `GNO_VERSION` in [ci.yml](.github/workflows/ci.yml). Install that release into the Makefile's toolchain store, `GNO_STORE`, replacing `<GNO_VERSION>` with the pin:

```sh
GOBIN=$HOME/.cache/gno-toolchains/onyx go install github.com/gnolang/gno/gnovm/cmd/gno@<GNO_VERSION>
make gno-deps
make test
```

`make gno-deps` fetches the realms' dependencies from onyx, once. `make e2e` runs the realms, tx-indexer and gnotifd together on a local chain. `make help` lists every target.

## Status

gnotif is at version 0 and targets the onyx testnet (`onyx-1`). A public server runs at gnotif.xyz. Version 0 leaves out:

- abuse limits: caps per IP, a send budget per declarer, and expiring subscriptions that stop re-registering;
- monitoring and a health endpoint;
- configuration through environment variables, beyond the VAPID keys;
- mainnet.

gnotif does not check that an address a browser opts in with belongs to that browser's user. A notification says only what the event already published on chain.

## License

Apache-2.0. See [LICENSE](LICENSE).
