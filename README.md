# <img src="site/static/favicon.svg" width="32" height="32" alt=""> gnotif

gnotif sends browser notifications when something happens on a gno.land chain, even when the page that asked for them is closed. A dapp's realm emits events, and a trigger declared in the gnotif registry realm says which event notifies whom. gnotifd, the gnotif server, reads the chain through [tx-indexer](https://github.com/gnolang/tx-indexer) and sends a Web Push for each match. The dapp's page subscribes the browser with the `gnotif` npm package.

Public onyx server: `https://gnotif.xyz/onyx`. Live demo: [demo.gnotif.xyz](https://demo.gnotif.xyz). Version 0, onyx testnet only.

The page calls gnotifd only to subscribe and to choose its triggers. The notification travels through the browser's push service and is shown by `sw.js`, the service worker the dapp serves from its own origin, so it arrives even when no tab of the dapp is open.

## Add notifications to a dapp

Your realm sets a trigger in the registry, from its `init`. This one notifies the address in each `Message` event's `to`:

```gno
h.Set(cross(me), "message", gnotif.Trigger{
	Event: "Message", Param: "to",
	Title: "New message", Body: "{msg}", Link: "/",
})
```

Your page opts the browser in with that address:

```js
const [message] = await gnotif.triggers("gno.land/r/<you>/notify");
await gnotif.enable();
await gnotif.setOptins([{ trigger: message.id, value: address }]);
```

[Getting started](docs/getting-started.md) builds the whole realm and page, and tries them on onyx.

## Run gnotifd

gnotif.xyz is a public gnotifd that any dapp can use. To run your own, follow [Running gnotifd](docs/running-gnotifd.md).

## Documentation

Each page also reads on the site, at [gnotif.xyz/docs](https://gnotif.xyz/docs/getting-started/).

- [Getting started](docs/getting-started.md): add notifications to a dapp, tried on onyx.
- [Use cases](docs/use-cases.md): triggers for one player, a board's readers or everyone, and a page that updates live.
- [Triggers](docs/triggers.md): the fields, templates and matching rules, the limit per realm, and who an event can notify.
- [Managing triggers](docs/managing-triggers.md): change a realm's triggers after its deploy, let admins change them, pause, revoke and drop.
- [Registry reference](docs/registry.md): the registry's functions, ids, namespace owners and panics.
- [How it works](docs/how-it-works.md): the path of a notification, the verified mark, how gnotifd reads the chain, delivery outcomes and the rules for a new registry version.
- [Browser client](js/README.md): the `gnotif` npm package and its service worker.
- [HTTP API](docs/http-api.md): the `/v1` endpoints and their errors.
- [Running gnotifd](docs/running-gnotifd.md): flags, VAPID keys, the push service allowlist, the indexer, backups and the container image.
- [Deploying the registry](docs/deploying-realms.md): deploy your own registry on onyx.
- [Protocols](docs/protocols.md): the registry's events, the rules for gnotif servers, and the remote store and hub contracts.
- [The demo](demo/README.md): echo, live at [demo.gnotif.xyz](https://demo.gnotif.xyz), sends you your own message as a notification. Run it on a local chain or deploy it to onyx. pingpong, a two-player example, sits beside it.

## Develop

`make test` runs the realm tests, the Go tests and the browser client's tests. It needs Go, Node.js and pnpm, which `nix develop` (or direnv, with the repo's `.envrc`) provides at the pinned versions. It installs the JavaScript dependencies itself. The first run installs the gno release onyx runs, pinned as `GNO_REF` in the [Makefile](Makefile), into `.cache/` and fetches the realms' dependencies from onyx. `make fclean` removes them.

`make e2e` runs the realms, tx-indexer and gnotifd together on a local chain. `make help` lists every target.

## Status

gnotif is at version 0 and runs on the onyx testnet (`onyx-1`) only. [Running gnotifd](docs/running-gnotifd.md#what-version-0-leaves-to-the-operator) lists what it leaves to the operator: health checks, abuse limits and monitoring.

## License

Apache-2.0. See [LICENSE](LICENSE).
