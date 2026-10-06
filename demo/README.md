# The pingpong demo

pingpong is a two-player game where the only move is to pass the turn. When your opponent passes you the turn, your browser shows "Your turn", even with the page closed. The demo runs at https://demo.gnotif.xyz, on the onyx testnet, with gnotif.xyz as its server.

This folder holds the realm and the page:

| Path | What it is |
|---|---|
| `gno.land/r/pingpong/v0/` | the realm |
| `index.html`, `app.mjs`, `address.mjs` | the page, a static dapp that opts the browser in with the `gnotif` client |
| `config.js` | the gnotif server, the realm and the gnoweb that the page uses |

## The realm

| Function | Who may call it | Event |
|---|---|---|
| `NewGame(cur, opponent address) string` | anyone | `GameInvited{game, first, second}` |
| `Accept(cur, game string)` | the invited player | `GameStarted{game, first, second}` |
| `Play(cur, game string)` | the player whose turn it is | `TurnPlayed{game, next, turn}` |
| `Leave(cur, game string)` | either player | `GameEnded{game, by}` |

pingpong declares its trigger from its `init` function, so deploying it declares the trigger with no other call:

```go
func init(cur realm) {
	gnotif.Declare(cross(cur), cur.PkgPath(), "TurnPlayed", "", "next", "Your turn", "Game {game}, turn {turn}", "/?game={game}")
}
```

A browser opts in with a player's address, and hears about the turns that pass to that address. A click on the notification opens the page at `/?game=<id>`, which links to the game on gnoweb.

No trigger watches `GameInvited`: anyone can invite any address, so a notification for it would let a stranger make that address's browser ring. `Play` refuses a game the opponent has not accepted, so a player's first notification comes after they accept.

## Run it on a local chain

This runs the whole stack on your machine: a local chain with gnodev, tx-indexer, gnotifd and the page. It needs Go, Git, Python 3 for `make demo`, and a browser with Web Push, such as Chrome. On macOS, also allow notifications for the browser in System Settings.

Run every command from the repository root. Steps 3, 5, 6 and 8 keep running, so give each of them its own terminal.

1. Build gnodev, gnokey and tx-indexer into `.tools/`:

   ```sh
   make tools
   ```

2. Make two player keys in `.tools/keys`, a keybase for this walkthrough. gnokey asks for a passphrase for each key, and `list` prints their addresses:

   ```sh
   .tools/gnokey add player1 -home .tools/keys
   .tools/gnokey add player2 -home .tools/keys
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

   - `-C /tmp` runs gnodev outside the repository. Inside it, gnodev would also load pingpong into the chain's genesis, and gnotifd cannot read genesis.
   - `-no-watch` keeps gnodev from restarting the chain when a file changes.

4. Deploy pingpong as player1. Its `init` declares its trigger in this same transaction:

   ```sh
   .tools/gnokey maketx addpkg -pkgpath gno.land/r/dev/pingpong/v0 -pkgdir demo/gno.land/r/pingpong/v0 \
     -gas-fee 1000000ugnot -gas-wanted 50000000 -home .tools/keys player1
   ```

   gnokey asks for player1's passphrase. The events it prints include `TriggerDeclared`, with `verified` set to `true`.

5. Start tx-indexer:

   ```sh
   .tools/tx-indexer start -db-path .tools/indexer-db -listen-address 127.0.0.1:8546
   ```

6. Start gnotifd:

   ```sh
   export $(go run ./cmd/gnotifd keygen)
   go run ./cmd/gnotifd -indexer http://127.0.0.1:8546/graphql/query \
     -registry gno.land/r/dev/gnotif/v0 -start-height 1 -max-age 1h \
     -vapid-subject <contact> -db .tools/gnotif.db
   ```

   - The first line makes the VAPID key pair that signs gnotifd's pushes, and keeps it in this terminal only. Restart gnotifd from this terminal with the second line alone: a new pair stops the pushes to the browser until you turn notifications off and on in the demo. [Running gnotifd](../docs/running-gnotifd.md#make-the-vapid-keys) keeps a pair in a file instead.
   - `<contact>` is where push services can reach you: any email address or https URL of yours.
   - gnodev makes a block only when a transaction arrives, so the first block after a quiet spell can carry an old time. gnotifd sends nothing for an event older than `-max-age`, which is 10 minutes by default and one hour here.

7. Open a game as player1 against player2, and accept it as player2. Replace `<player2 address>` with the address that `gnokey list` printed:

   ```sh
   .tools/gnokey maketx call -pkgpath gno.land/r/dev/pingpong/v0 -func NewGame -args <player2 address> \
     -gas-fee 1000000ugnot -gas-wanted 50000000 -home .tools/keys player1
   .tools/gnokey maketx call -pkgpath gno.land/r/dev/pingpong/v0 -func Accept -args 0000001 \
     -gas-fee 1000000ugnot -gas-wanted 50000000 -home .tools/keys player2
   ```

   gnotifd now lists pingpong's trigger, with `"verified":true`:

   ```sh
   curl -s 'http://127.0.0.1:8080/v1/triggers?target=gno.land/r/dev/pingpong/v0'
   ```

8. Serve the demo:

   ```sh
   make demo
   ```

   Open http://localhost:3000, paste player2's address, click "Turn on notifications", and allow notifications when the browser asks.

9. Play a turn as player1:

   ```sh
   .tools/gnokey maketx call -pkgpath gno.land/r/dev/pingpong/v0 -func Play -args 0000001 \
     -gas-fee 1000000ugnot -gas-wanted 50000000 -home .tools/keys player1
   ```

   Within about 15 seconds, the browser shows "Your turn" with "Game 0000001, turn 1", even with the demo's tab closed. With the tab closed, a click opens the demo at `/?game=0000001`, which links to the game on gnoweb.

Turns alternate. A turn that player1 plays notifies this browser. A turn that player2 plays does not, since this browser opted in with player2's address only.

gnodev keeps its chain in memory, so a restarted gnodev starts an empty chain. To start again, stop gnodev, tx-indexer and gnotifd, and delete tx-indexer's and gnotifd's databases:

```sh
rm -rf .tools/indexer-db .tools/gnotif.db*
```

Then repeat steps 3 to 7, and turn notifications off and on in the demo, so that the new database learns this browser's subscription.

The demo's "Following" count is the page's own record of the opt-ins it last sent, kept in the browser's local storage, because the server cannot read opt-ins back. `make clean` removes `.tools/`, with the tools, the keys and the databases.

## Deploy it to onyx

1. Deploy a registry and start a gnotifd that reads it, as [Deploying the registry](../docs/deploying-realms.md) shows. Its first step, `make deploy-pkgs NS=gno.land/r/<namespace>`, also writes pingpong's copy to `.tools/deploy/pingpong/v0`, importing your registry, and its lint step checks both copies.
2. Once the registry is `live`, deploy `.tools/deploy/pingpong/v0` at `gno.land/r/<namespace>/pingpong/v0` with `gno_addpkg`, and wait for `live`. onyx enables a new package a few seconds after its deploy, and runs pingpong's `init` then, so the trigger is declared with no other call.
3. Check that the registry lists the trigger as verified, on gnoweb at `https://onyx.testnets.gno.land/r/<namespace>/gnotif/v0:target/gno.land/r/<namespace>/pingpong/v0`.
4. Point the page at the deploy in `config.js`: `server` to your gnotifd's URL, `pingpong` to `gno.land/r/<namespace>/pingpong/v0`, and `gnoweb` to `https://onyx.testnets.gno.land`.
5. Serve this folder at the root of a site, with `gnotif.js` and `sw.js` copied from `js/src/`, as `make demo` does.
