# Getting started

This guide adds browser notifications to a dapp in three steps: the realm emits an event, a trigger in the gnotif registry says who that event notifies, and the dapp's page opts browsers in with the `gnotif` npm package. pingpong, the demo game in `gno/r/pingpong/v0`, is the running example, and the last section runs it on a local chain with the demo dapp.

The realm paths here are the repository's: `gno.land/r/dev/gnotif/v0` for the registry and `gno.land/r/dev/pingpong/v0` for pingpong. A deployed copy lives under its deployer's namespace ([Deploying the realms](deploying-realms.md)). Import the registry by the path it is deployed at.

## 1. Emit an event

A realm emits an event with `chain.Emit(type, key, value, ...)`. The chain stamps the event with the realm's package path, so no other realm can emit under that path. pingpong names the next player on every turn:

```go
chain.Emit("TurnPlayed", "game", g.ID, "next", g.Next.String(), "turn", strconv.Itoa(g.Turn))
```

- A trigger names the event type, and may name attribute keys for its parameter and its filter. Each of these names is 1 to 64 bytes of `A-Z a-z 0-9 _`, so emit those attributes under such keys.
- gnotif reads only the events of successful transactions.
- When an attribute key repeats in one event, gnotif uses its first value.

### Name a player only after they agree

Anyone can call `NewGame` with any address, so an event that names the invited player would let a stranger make that player's browser ring. pingpong names an address in a notifying event only after that address has acted:

| Function | Who may call | Event |
|---|---|---|
| `NewGame(cur, opponent address) string` | anyone | `GameInvited{game, first, second}` |
| `Accept(cur, game string)` | the invited player | `GameStarted{game, first, second}` |
| `Play(cur, game string)` | the player whose turn it is | `TurnPlayed{game, next, turn}` |
| `Leave(cur, game string)` | either player | `GameEnded{game, by}` |

No trigger watches `GameInvited`, and `Play` refuses a game the opponent has not accepted, so the first `TurnPlayed` comes after `Accept`. Follow the same rule in your realm: name an address in an event a trigger watches only once that address has accepted, joined or otherwise acted.

## 2. Declare a trigger

The registry has two functions:

```go
func Declare(cur realm, target, event, filter, param, title, body, link string) string
func Remove(cur realm, id string)
```

`Declare` stores a trigger and returns its id. `Remove` deletes one, and only the trigger's declarer may call it.

### Declare from the realm itself

A trigger is verified when the caller of `Declare` is the target realm itself. pingpong declares its trigger this way:

```go
import "gno.land/r/dev/gnotif/v0"

var declared bool

// DeclareTriggers declares the "your turn" trigger in the gnotif registry.
func DeclareTriggers(cur realm) {
	if declared {
		panic("triggers already declared")
	}
	declared = true
	gnotif.Declare(cross(cur), cur.PkgPath(), "TurnPlayed", "", "next",
		"Your turn", "Game {game}, turn {turn}", "/?game={game}")
}
```

- `cross(cur)` makes pingpong the caller the registry sees, so the trigger is verified.
- `cur.PkgPath()` passes pingpong's own path as the target, which stays right under any deploy path.
- Anyone may call `DeclareTriggers`, and the `declared` flag makes it run once. Call it once after the deploy.
- The declarer of this trigger is pingpong's address, so only pingpong could remove it, through a function that calls `gnotif.Remove(cross(cur), id)`. pingpong has none.

An account can also call `Declare` directly, as a transaction. Such a trigger works the same way and is listed without the verified mark.

The verified mark means "declared by the realm at the target path". It holds because the chain lets only the owner of a namespace deploy under it ([How it works](how-it-works.md#what-the-verified-mark-means)).

### Fields

| Field | Meaning | Rule |
|---|---|---|
| `target` | package path of the realm whose events the trigger watches | starts with `gno.land/r/`, at most 256 bytes |
| `event` | event type to match | 1 to 64 bytes of `A-Z a-z 0-9 _` |
| `filter` | fixed `key=value` pairs the event must carry, comma-separated; empty for none | up to 8 pairs; each key 1 to 64 bytes of `A-Z a-z 0-9 _`; each value 1 to 64 bytes without `=` or `,` |
| `param` | attribute whose value a browser opts in with; empty notifies every browser that opted in | empty, or 1 to 64 bytes of `A-Z a-z 0-9 _` |
| `title` | notification title template | 1 to 64 bytes |
| `body` | notification body template | at most 255 bytes |
| `link` | path template opened on click, on the dapp's own origin | 1 to 256 bytes; starts with `/`; does not start with `//`; no byte below `0x20`, no `0x7F` and no backslash |

`Declare` panics on the first field that breaks its rule, which reverts the transaction.

Names are limited to identifier characters so that a declarer cannot make a rendered trigger imitate the verified mark. The link is a path because anyone can declare a trigger on any realm: a full URL would let a stranger send a dapp's users to another site. A path always resolves against the origin of the dapp whose page subscribed the browser.

### Templates

In `title`, `body` and `link`, `{key}` stands for the value of the event's attribute `key`.

- A missing attribute renders as an empty string, and a `{` without a closing `}` is copied as is.
- In `link`, every value is percent-escaped, so a value cannot add a path segment or a host. `/?game={game}` renders as `/?game=0000001`.
- gnotifd cuts a rendered title to 64 bytes and a rendered body to 255 bytes, at a character boundary. A rendered link longer than 1,024 bytes becomes `/`.

### Matching

An event matches a trigger when:

1. its package path equals `target`, and its type equals `event`;
2. it carries every `filter` pair with the same value;
3. when `param` is set, its attribute `param` equals the value the browser opted in with. An event without that attribute notifies no one.

pingpong's trigger has the param `next`, so a browser that opts in with an address hears about the turns that pass to that address.

### See the triggers

The registry renders on gnoweb. Its main page lists the triggers newest first, 50 to a page: `/r/dev/gnotif/v0`, then `/r/dev/gnotif/v0:page/2`. `/r/dev/gnotif/v0:trigger/<id>` shows one trigger with its templates. Trigger ids count up from `0000001`.

## 3. Add the client to the page

Install the package:

```sh
npm install gnotif
```

Serve a copy of its service worker from the dapp's origin, for example from the folder your site serves at its root:

```sh
cp node_modules/gnotif/src/sw.js public/sw.js
```

A service worker controls only the origin that serves it, so the dapp serves `sw.js` itself, and no code from the gnotif server's operator runs on the dapp's origin. Copy it again after each upgrade.

Then subscribe the browser and opt it in:

```js
import { Gnotif, GnotifError } from "gnotif";

const gnotif = new Gnotif({ server: "https://gnotif.example" });

const yourTurn = (await gnotif.triggers()).find(
  (t) => t.target === "gno.land/r/dev/pingpong/v0" && t.event === "TurnPlayed" && t.verified,
);

button.addEventListener("click", async () => {
  try {
    await gnotif.enable();
    await gnotif.setOptins([{ trigger: yourTurn.id, value: playerAddress }]);
  } catch (err) {
    if (err instanceof GnotifError && err.code === "denied") {
      showMessage("Allow notifications for this site to get them.");
    } else {
      throw err;
    }
  }
});
```

- Find a trigger by its target, event and verified mark rather than by id: each registry numbers its own triggers.
- `enable()` asks for notification permission, so call it from a click. It registers `sw.js`, subscribes the browser with the server's key and registers the subscription with the server.
- `setOptins()` replaces the browser's whole set of opt-ins. A trigger with a param takes a value, and one without takes `""`.
- The server has no route that reads opt-ins back. Keep the set your page last sent, and send the whole set again on each change.
- A click on the notification opens the trigger's link on the dapp's origin, here `/?game=0000001`, or focuses a tab of the dapp that is already open ([the client's README](../js/README.md#notifications)). Read the link from `location.search` to greet the player.
- `enabled()` says whether this browser is subscribed and still allowed to show notifications, and `disable()` turns notifications off.

[The client's README](../js/README.md) documents each method and its errors.

gnotif does not check that an address belongs to the browser that opts in with it: anyone can ask for pingpong's "Your turn" for any address. A notification says only what the event already published on chain.

## Try it on a local chain

The demo in `demo/` is a static dapp built this way. It runs on its own origin and calls gnotifd cross-origin, as a real dapp does. `demo/config.js` names what it talks to:

| Export | Value | What it is |
|---|---|---|
| `server` | `http://localhost:8080` | the gnotif server |
| `pingpong` | `gno.land/r/dev/pingpong/v0` | the realm whose trigger the page leads with |
| `gnoweb` | `http://localhost:8888` | the gnoweb that renders the games |

Run the chain, tx-indexer, gnotifd and the demo on your machine, each in its own terminal, from the repository root. This needs Go, Git, Python 3 for `make demo`, and a browser with Web Push, such as Chrome. On macOS, allow notifications for the browser in System Settings.

1. Build gnodev, gnokey and tx-indexer into `.tools/`:

   ```sh
   make tools GNO_CHECKOUT=https://github.com/gnolang/gno
   ```

2. Make two player keys in a throwaway keybase. gnokey asks for a passphrase for each:

   ```sh
   .tools/gnokey add player1 -home .tools/keys
   .tools/gnokey add player2 -home .tools/keys
   .tools/gnokey list -home .tools/keys
   ```

3. Start the chain. gnodev loads both realms from the `gno/` workspace, funds every key in `.tools/keys`, and serves gnoweb on http://localhost:8888:

   ```sh
   GNOROOT=$PWD/.tools/gno-src .tools/gnodev local -C gno -no-watch -home $PWD/.tools/keys
   ```

4. Start tx-indexer on the chain:

   ```sh
   .tools/tx-indexer start -remote http://127.0.0.1:26657 -db-path .tools/indexer-db -listen-address 127.0.0.1:8546
   ```

5. Start gnotifd on the indexer:

   ```sh
   go run ./cmd/gnotifd keygen > .tools/vapid.env
   set -a; . ./.tools/vapid.env; set +a
   go run ./cmd/gnotifd -indexer http://127.0.0.1:8546/graphql/query \
     -registry gno.land/r/dev/gnotif/v0 -start-height 1 -max-age 1h \
     -vapid-subject https://gno.land/r/dev/gnotif/v0 -db .tools/gnotif.db
   ```

6. Declare pingpong's trigger, then open a game and accept it. Replace `<player2 address>` with the address `gnokey list` printed:

   ```sh
   .tools/gnokey maketx call -pkgpath gno.land/r/dev/pingpong/v0 -func DeclareTriggers \
     -gas-fee 1000000ugnot -gas-wanted 50000000 -broadcast -chainid dev \
     -remote 127.0.0.1:26657 -home .tools/keys player1
   .tools/gnokey maketx call -pkgpath gno.land/r/dev/pingpong/v0 -func NewGame -args <player2 address> \
     -gas-fee 1000000ugnot -gas-wanted 50000000 -broadcast -chainid dev \
     -remote 127.0.0.1:26657 -home .tools/keys player1
   .tools/gnokey maketx call -pkgpath gno.land/r/dev/pingpong/v0 -func Accept -args 0000001 \
     -gas-fee 1000000ugnot -gas-wanted 50000000 -broadcast -chainid dev \
     -remote 127.0.0.1:26657 -home .tools/keys player2
   ```

   After these calls, `curl -s http://127.0.0.1:8080/v1/triggers` lists the trigger with `"verified":true`.

7. Serve the demo, open http://localhost:3000, paste player2's address, click "Turn on notifications", and allow notifications when the browser asks:

   ```sh
   make demo
   ```

8. Play a turn as player1. Within about ten seconds the browser shows "Your turn" with "Game 0000001, turn 1", even with the demo's tab closed. With the tab closed, a click opens the demo at `/?game=0000001`, which links to the game on gnoweb.

   ```sh
   .tools/gnokey maketx call -pkgpath gno.land/r/dev/pingpong/v0 -func Play -args 0000001 \
     -gas-fee 1000000ugnot -gas-wanted 50000000 -broadcast -chainid dev \
     -remote 127.0.0.1:26657 -home .tools/keys player1
   ```

   Turns alternate. A turn player1 plays notifies this browser, and a turn player2 plays does not, since this browser opted in with player2's address only.

Things to know about this setup:

- `-max-age 1h` keeps a turn deliverable after a quiet spell of up to an hour. gnodev makes a block only when a transaction arrives, so the first event after an idle period can carry a block time from before it, and gnotifd skips events older than `-max-age`, 10 minutes by default.
- gnodev keeps its chain in memory. After restarting it, delete `.tools/indexer-db` and `.tools/gnotif.db*` before starting tx-indexer and gnotifd again, then turn notifications off and on in the demo so the new database learns the subscription.
- The demo's "Following" count is the page's own record of the opt-ins it last sent, kept in the browser's local storage, because the server cannot read opt-ins back.
- `make clean` removes `.tools/`, keys and databases included.
