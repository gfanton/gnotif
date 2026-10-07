# The echo demo

echo sends you your own message as a browser notification: one person, one message, one notification. You turn on notifications for your gno.land address, call the echo realm with a message, and the browser shows it, even with the page closed. The page sends the message with the Adena wallet, or gives you a gnokey command to run. The deployed demo runs at https://demo.gnotif.xyz, on the onyx testnet, with gnotif.xyz as its server.

This folder holds the realms and the page:

| Path | What it is |
|---|---|
| `gno.land/r/echo/v0/` | the echo realm, which the page calls |
| `gno.land/r/pingpong/v0/` | pingpong, an example of a two-player dapp |
| `index.html`, `app.mjs`, `address.mjs`, `adena.mjs`, `echo-tx.mjs`, `optin.mjs` | the page, a static dapp that opts the browser in with the `gnotif` client |
| `config.js` | the gnotif server, the echo realm, the chain and the gnoweb that the page uses |

## The echo realm

echo has one function to call, `Echo(cur, msg string)`. It emits `Echo{to, msg}`, where `to` is the caller's address. echo declares its trigger from its `init` function, so deploying it declares the trigger with no other call:

```go
func init(cur realm) {
	gnotif.Declare(cross(cur), cur.PkgPath(), "Echo", "", "to", "Echo", "{msg}", "/")
}
```

A browser opts in with an address, and shows each message that address sends to echo, with "Echo" as the title and the message as the body. A click on the notification opens the demo.

The recipient is always the caller and never an argument, so nobody can notify another address ([Who an event can notify](../docs/triggers.md#who-an-event-can-notify)).

## Run it on a local chain

This runs the whole stack on your machine: a local chain with gnodev, tx-indexer, gnotifd and the page. It needs Go, Git, Python 3 for `make demo`, and a browser with Web Push, such as Chrome. On macOS, also allow notifications for the browser in System Settings.

Run every command from the repository root. Steps 3, 5, 6 and 7 keep running, so give each of them its own terminal.

1. Build gnodev, gnokey and tx-indexer into `.tools/`:

   ```sh
   make tools
   ```

2. Make a key named `me` in `.tools/keys`, a keybase for this walkthrough. gnokey asks for a passphrase, and `list` prints the key's address:

   ```sh
   .tools/gnokey add me -home .tools/keys
   .tools/gnokey list -home .tools/keys
   ```

3. Start the chain with the gnotif registry:

   ```sh
   .tools/gnodev local -C /tmp -no-watch -home $PWD/.tools/keys \
     $PWD/gno/r/gnotif/v0 \
     $PWD/.tools/gno-src/examples/gno.land/p/nt/avl/v0 \
     $PWD/.tools/gno-src/examples/gno.land/p/nt/cford32/v0 \
     $PWD/.tools/gno-src/examples/gno.land/p/nt/seqid/v0 \
     $PWD/.tools/gno-src/examples/gno.land/p/nt/markdown/sanitize/v0
   ```

   gnodev loads the registry and the packages it depends on, gives test GNOT to every key in `.tools/keys`, and serves gnoweb on http://localhost:8888.

   - `-C /tmp` runs gnodev outside the repository. Inside it, gnodev would also load echo into the chain's genesis, and gnotifd cannot read genesis.
   - `-no-watch` keeps gnodev from restarting the chain when a file changes.

4. Deploy echo with your key. Its `init` declares its trigger in this same transaction:

   ```sh
   .tools/gnokey maketx addpkg -pkgpath gno.land/r/dev/echo/v0 -pkgdir demo/gno.land/r/echo/v0 \
     -gas-fee 1000000ugnot -gas-wanted 50000000 -home .tools/keys me
   ```

   gnokey asks for the passphrase. The events it prints include `TriggerDeclared`, with `verified` set to `true`.

5. Start tx-indexer:

   ```sh
   .tools/tx-indexer start -db-path .tools/indexer-db -listen-address 127.0.0.1:8546
   ```

6. Make the VAPID key pair that signs gnotifd's pushes, in a file that only you can read. Run this block once:

   ```sh
   touch .tools/vapid.env
   chmod 600 .tools/vapid.env
   go run ./cmd/gnotifd keygen > .tools/vapid.env
   ```

   Running it again makes a new pair, and the browser then gets no notification until you turn notifications off and on in the demo.

   Load the pair and start gnotifd. To restart gnotifd, run this block only:

   ```sh
   export $(cat .tools/vapid.env)
   go run ./cmd/gnotifd -indexer http://127.0.0.1:8546/graphql/query \
     -registry gno.land/r/dev/gnotif/v0 -start-height 1 -max-age 1h \
     -vapid-subject <contact> -db .tools/gnotif.db
   ```

   - `<contact>` is where push services can reach you: any email address or https URL of yours.
   - `-max-age 1h` lets gnotifd notify for gnodev's blocks, whose time can be old ([Running gnotifd](../docs/running-gnotifd.md#the-indexer-and-the-start-height)).

   Within a few seconds, gnotifd lists echo's trigger, with `"verified":true`:

   ```sh
   curl -s 'http://127.0.0.1:8080/v1/triggers?target=gno.land/r/dev/echo/v0'
   ```

7. Serve the demo:

   ```sh
   make demo
   ```

   Open http://localhost:3000, paste the address that `gnokey list` printed, click "Turn on notifications", and allow notifications when the browser asks.

8. Under "Send yourself a message", the page shows a gnokey command that sends the message in the field, or "Hello, me" when the field is empty. Run it with `.tools/gnokey` in place of `gnokey`, and `-home .tools/keys me` in place of `<your key>`:

   ```sh
   .tools/gnokey maketx call -pkgpath gno.land/r/dev/echo/v0 -func Echo -args 'Hello, me' \
     -gas-wanted 10000000 -gas-fee 20000ugnot -chainid dev -remote http://127.0.0.1:26657 -home .tools/keys me
   ```

   gnokey asks for the passphrase. Within about 15 seconds, the browser shows "Echo" with "Hello, me", even with the demo's tab closed. A click opens the demo.

gnodev keeps its chain in memory, so a restarted gnodev starts an empty chain. To start again, stop gnodev, tx-indexer and gnotifd, and delete tx-indexer's and gnotifd's databases:

```sh
rm -rf .tools/indexer-db .tools/gnotif.db*
```

Then repeat steps 3 to 6 without the block that makes the VAPID keys, and turn notifications off and on in the demo, so that the new database learns this browser's subscription.

`make clean` removes `.tools/`, with the tools, the keys and the databases.

## Deploy it to onyx

1. Deploy a registry and start a gnotifd that reads it, as [Deploying the registry](../docs/deploying-realms.md) shows. Its first step, `make deploy-pkgs NS=gno.land/r/<namespace>`, also writes echo's copy to `.tools/deploy/echo/v0`, importing your registry, and its lint step checks echo too.
2. Once the registry's page opens on gnoweb, deploy echo with the same key. gnokey asks for your passphrase:

   ```sh
   gnokey maketx addpkg -pkgpath gno.land/r/<namespace>/echo/v0 -pkgdir .tools/deploy/echo/v0 \
     -gas-wanted 40000000 -gas-fee 80000ugnot -max-deposit 20000000ugnot \
     -chainid onyx-1 -remote https://rpc.onyx.testnets.gno.land:443 mykey
   ```

   onyx enables echo a few seconds after its deploy, and runs its `init` then, so the trigger is declared with no other call.
3. Check that the registry lists the trigger as verified, on gnoweb at `https://onyx.testnets.gno.land/r/<namespace>/gnotif/v0:target/gno.land/r/<namespace>/echo/v0`.
4. Point the page at the deploy in `config.js`, whose comment gives the onyx values: `server` to your gnotifd's URL, `echo` to `gno.land/r/<namespace>/echo/v0`, `chain` to onyx's id, name and RPC, and `gnoweb` to onyx's gnoweb.
5. Serve this folder at the root of a site, with `gnotif.js` and `sw.js` copied from `js/src/`, as `make demo` does.

## pingpong, a two-player example

pingpong is a game where the only move is to pass the turn. It shows what echo does not: an event that notifies another address, which it names only after that address accepts the game.

| Function | Who may call it | Event |
|---|---|---|
| `NewGame(cur, opponent address) string` | anyone | `GameInvited{game, first, second}` |
| `Accept(cur, game string)` | the invited player | `GameStarted{game, first, second}` |
| `Play(cur, game string)` | the player whose turn it is | `TurnPlayed{game, next, turn}` |
| `Leave(cur, game string)` | either player | `GameEnded{game, by}` |

Its `init` declares one trigger, on `TurnPlayed`, whose param `next` is the player who plays next:

```go
func init(cur realm) {
	gnotif.Declare(cross(cur), cur.PkgPath(), "TurnPlayed", "", "next", "Your turn", "Game {game}, turn {turn}", "/?game={game}")
}
```

No trigger watches `GameInvited`, since anyone can invite any address ([Who an event can notify](../docs/triggers.md#who-an-event-can-notify)). No player hears of a game before the invited player accepts it, since `Play` refuses a game that has not started.

The test in [`e2e/`](../e2e) runs pingpong on a local chain with tx-indexer and gnotifd: one player opens a game, the other accepts, and only the turns passed to the opted-in player send a push. `make e2e` runs it.
