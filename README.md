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

These steps use the repository's placeholder paths under `gno.land/r/dev`. A deployed registry has its own path, under the namespace of whoever deployed it.

1. **Emit an event** from your realm. pingpong, the demo game in `gno/r/pingpong/v0`, names the next player on every turn:

   ```go
   chain.Emit("TurnPlayed", "game", g.ID, "next", g.Next.String(), "turn", strconv.Itoa(g.Turn))
   ```

2. **Declare a trigger** in the registry. Declared by the realm itself, the trigger is verified:

   ```go
   import "gno.land/r/dev/gnotif/v0"

   var declared bool

   func DeclareTriggers(cur realm) {
   	if declared {
   		panic("triggers already declared")
   	}
   	declared = true
   	gnotif.Declare(cross(cur), cur.PkgPath(), "TurnPlayed", "", "next",
   		"Your turn", "Game {game}, turn {turn}", "/?game={game}")
   }
   ```

   This trigger notifies the browsers that opted in with the address in the event's `next` attribute. A click opens `/?game=<id>` on the dapp's own origin.

3. **Add the client** to the dapp's page, and serve a copy of its service worker from the dapp's origin:

   ```sh
   npm install gnotif
   cp node_modules/gnotif/src/sw.js public/sw.js
   ```

   ```js
   import { Gnotif } from "gnotif";

   const gnotif = new Gnotif({ server: "https://gnotif.example" });
   const yourTurn = (await gnotif.triggers()).find(
     (t) => t.target === "gno.land/r/dev/pingpong/v0" && t.event === "TurnPlayed" && t.verified,
   );
   if (yourTurn === undefined) {
     throw new Error("This gnotif server does not offer pingpong's trigger.");
   }

   button.addEventListener("click", async () => {
     await gnotif.enable();
     await gnotif.setOptins([{ trigger: yourTurn.id, value: playerAddress }]);
   });
   ```

   `enable()` asks for permission, so call it from a click.

[Getting started](docs/getting-started.md) covers each step in full and runs the whole loop on a local chain with the demo dapp in `demo/`.

## Run gnotifd

```sh
go install github.com/gfanton/gnotif/cmd/gnotifd@latest
(umask 077; gnotifd keygen > vapid.env)
set -a; . ./vapid.env; set +a
gnotifd -indexer https://indexer.onyx.testnets.gno.land/graphql/query \
  -registry gno.land/r/<namespace>/gnotif/v0 -start-height <height> \
  -vapid-subject <contact>
```

`vapid.env` holds the private key that signs every push: keep it outside any source checkout. `<contact>` is an email address or https URL where push services can reach you, such as `ops@example.org`. gnotifd serves its API on `127.0.0.1:8080` and keeps its state in `gnotif.db`. `-start-height` is a height at or before the registry's deploy, and only the first start needs it. [Running gnotifd](docs/running-gnotifd.md) covers the flags, the keys, the push service allowlist and the container image.

## Documentation

- [Getting started](docs/getting-started.md): from realm to page, and the demo on a local chain.
- [How it works](docs/how-it-works.md): the watch window, storage, delivery outcomes and the rules for gnotif servers.
- [Running gnotifd](docs/running-gnotifd.md): flags, VAPID keys, the push service allowlist, the indexer and the container image.
- [HTTP API](docs/http-api.md): the `/v1` endpoints, errors and CORS.
- [Deploying the realms](docs/deploying-realms.md): deploy copies, linting and the deploy order on onyx.
- [Browser client](js/README.md): the `gnotif` npm package.

## Develop

`make test` runs the realm tests, the Go tests and the browser client's tests. It needs Go, Node.js and the gno toolchain of the release onyx runs, which CI pins as `GNO_VERSION` in [ci.yml](.github/workflows/ci.yml). Install that release into the Makefile's toolchain store, `GNO_STORE`, replacing `<GNO_VERSION>` with the pin:

```sh
GOBIN=$HOME/.cache/gno-toolchains/onyx go install github.com/gnolang/gno/gnovm/cmd/gno@<GNO_VERSION>
make gno-deps
make test
```

`make gno-deps` fetches the realms' dependencies from onyx, once. `make e2e GNO_CHECKOUT=https://github.com/gnolang/gno` runs the realms, tx-indexer and gnotifd together on a local chain. `make help` lists every target.

## Status

gnotif is at version 0 and targets the onyx testnet (`onyx-1`). Version 0 leaves out:

- abuse limits: caps per IP, a send budget per declarer, and expiring subscriptions that stop re-registering;
- a hosted public instance;
- monitoring and a health endpoint;
- configuration through environment variables, beyond the VAPID keys;
- mainnet.

gnotif does not check that an address a browser opts in with belongs to that browser's user.

## License

Apache-2.0. See [LICENSE](LICENSE).
