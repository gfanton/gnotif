# <img src="site/static/favicon.svg" width="32" height="32" alt=""> gnotif

gnotif sends browser notifications when something happens on a gno.land chain, even when the page that asked for them is closed. A dapp's realm emits events, and a trigger declared in the gnotif registry realm says which event notifies whom. gnotifd, the gnotif server, reads the chain through [tx-indexer](https://github.com/gnolang/tx-indexer) and sends a Web Push for each match. The dapp's page subscribes the browser with the `gnotif` npm package.

Public onyx server: `https://gnotif.xyz/onyx`. Live demo: [demo.gnotif.xyz](https://demo.gnotif.xyz). Version 0, onyx testnet only.

The page calls gnotifd only to subscribe and to choose its triggers. The notification travels through the browser's push service and is shown by `sw.js`, the service worker the dapp serves from its own origin, so it arrives even when no tab of the dapp is open.

## Add notifications to a dapp

Your realm declares a trigger and your page opts the browser in. [Getting started](docs/getting-started.md) shows both and tries them on onyx.

## Run gnotifd

gnotif.xyz is a public gnotifd that any dapp can use. To run your own, follow [Running gnotifd](docs/running-gnotifd.md).

## Documentation

- [Getting started](docs/getting-started.md): add notifications to a dapp, tried on onyx.
- [Triggers](docs/triggers.md): the fields, templates and matching rules, the limit per realm, and who an event can notify.
- [How it works](docs/how-it-works.md): the verified mark, how gnotifd reads the chain, delivery outcomes and the rules for gnotif servers.
- [Browser client](js/README.md): the `gnotif` npm package and its service worker.
- [HTTP API](docs/http-api.md): the `/v1` endpoints and their errors.
- [Running gnotifd](docs/running-gnotifd.md): flags, VAPID keys, the push service allowlist, the indexer, backups and the container image.
- [Deploying the registry](docs/deploying-realms.md): deploy your own registry on onyx.
- [The demo](demo/README.md): echo, live at [demo.gnotif.xyz](https://demo.gnotif.xyz), sends you your own message as a notification. Run it on a local chain or deploy it to onyx. pingpong, a two-player example, sits beside it.

## Develop

`make test` runs the realm tests, the Go tests and the browser client's tests. It needs Go and Node.js. The first run installs the gno release onyx runs, pinned as `GNO_REF` in the [Makefile](Makefile), into `.cache/` and fetches the realms' dependencies from onyx. `make fclean` removes them.

`make e2e` runs the realms, tx-indexer and gnotifd together on a local chain. `make help` lists every target.

## Status

gnotif is at version 0 and runs on the onyx testnet (`onyx-1`) only. [Running gnotifd](docs/running-gnotifd.md#what-version-0-leaves-to-the-operator) lists what it leaves to the operator: health checks, abuse limits and monitoring.

## License

Apache-2.0. See [LICENSE](LICENSE).
