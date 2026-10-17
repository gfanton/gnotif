# How gnotif works

gnotifd is one process with one database, a SQLite file or a [remote store](running-gnotifd.md#remote-store). A watch loop reads realm events from tx-indexer and hands a push to the sender for every opt-in an event matches. The sender sends from memory, on one routine per push service. An HTTP server takes the browsers' subscriptions and opt-ins, and holds the pages' live connections. The watch loop never waits for a push, so a slow push service never holds back reading the chain. A crash loses the pushes still in memory ([Delivery outcomes](#delivery-outcomes)).

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

## Triggers come from the registry's events

gnotifd never reads the registry's state. It rebuilds its triggers from the events the registry emits:

| Event | Attributes |
|---|---|
| `TriggerDeclared` | `id`, `target`, `event`, `filter`, `param`, `title`, `body`, `link`, `declarer`, `verified` (always `true` in v0) |
| `TriggerRemoved` | `id` |

gnotifd applies these events only when they come from the registry version given with `-registry` or a later version under the same prefix, such as `…/gnotif/v1` after `…/gnotif/v0`. It never reads an earlier version. An event applies only when its id carries its own version's prefix: `0000001` in v0, `v<N>-…` in version N. It logs any other event as `skip registry event` and skips it. It stores a trigger only when `verified` is `true`. It logs a `TriggerDeclared` it cannot read as an error and skips it. A `TriggerRemoved` deletes the trigger with its opt-ins, but a push the trigger already queued is still sent. Because the trigger set comes from the chain alone, any operator can run a server against the same registry and get the same triggers.

[Triggers](triggers.md) covers each field and the [limit of 64 per realm and version](triggers.md#at-most-64-triggers-per-realm).

## What the verified mark means

`Declare` panics with `caller is not the target` unless its caller is code whose package path equals the trigger's target. So only a realm can declare its own triggers, normally from its `init`, and every stored trigger carries the verified mark: "declared by the realm at the target path".

The mark rests on the chain refusing deploys under a namespace to anyone but its owner. On onyx, a key deploys under its own address, `gno.land/r/<address>/...`, or under a name it registered. On a chain without that rule, whoever deploys a realm at a path can declare triggers for that path.

A realm that hands its live `cur` to another realm lets that code declare triggers in the realm's name. Passing `cur` to a foreign crossing function is refused by the VM, with `cannot cur-call to external realm function`; an ordinary call whose realm argument is not the first one takes it, and that is the shape `chain/runtime/unsafe` recommends for a helper with no `cur` of its own, "accept `_ int, rlm realm` from the caller":

```go
// another realm, which your realm imports for convenience
func Setup(tag string, rlm realm) string {
	return gnotif.Declare(cross(rlm), rlm.PkgPath(), "Message", "", "to",
		"Wallet alert", "Your account is at risk: {msg}", "/drain")
}

// yours
func init(cur realm) { other.Setup("x", cur) }
```

The trigger that lands is verified, its target is your realm, and its declarer is your realm's address, so only your realm may `Remove` it: one declared from `init` by a realm that exports no removal stays for good. A helper called more than once also fills your 64 places, after which your own `Declare` panics with `too many triggers for target`. Declare your triggers from your own source, and read a `realm` in another realm's argument list as the authority it is.

The registry bounds what any trigger can do:

- its names are identifiers and its target holds only the characters of a realm path, so its rendered text cannot imitate the mark;
- its link is a path, which always opens on the dapp's own origin, so a stranger's trigger cannot send the dapp's users to another site;
- the registry's pages sanitize the text a declarer supplied.

## How gnotifd reads the chain

gnotifd polls the indexer every `-poll` interval, 5 seconds by default.

- It reads up to 1,000 blocks at a time, from where it stopped. One request returns every realm event in those blocks, and gnotifd matches the events to its triggers itself.
- After a failure, it reads the same blocks again with a smaller window.
- It reads the triggers from the database once, at start, and keeps them in memory. Matching an event and checking an opt-in make no database call. A direct edit of the triggers table shows after a restart.
- A window of blocks that holds a registry event or a match is committed in one database call, together with the position. Any other window saves its position every 10 minutes, so a restart can read again what it read in the last 10 minutes.
- It skips a single transaction too large to read, logs an error, and loses only that transaction's events ([Running gnotifd](running-gnotifd.md#the-indexer-and-the-start-height)).

## Storage

The database holds the triggers, the subscriptions with their opt-ins, the chain position, and the outbox: the pushes waiting for a retry. [Running gnotifd](running-gnotifd.md#storage-and-backups) covers its backups and when to start a new one.

## Delivery outcomes

The sender gives each push to the routine of its push service: one per `-push-hosts` entry, and one per host for an endpoint that matches no entry. A routine sends its pushes one at a time, in the order it got them, so a slow service delays only its own pushes. The pushes go out from memory, and a push that succeeds writes nothing to the database. At most 10,000 pushes are held in memory, queued or being sent, across all services. A push past that goes to the outbox.

At start and every second, the sender reads the outbox's pushes whose next attempt is due, oldest first, 100 at a time, and gives them to the routines while memory has room.

A push carries `{"title", "body", "link"}` as JSON, encrypted for the subscription and signed with the VAPID key. Push services keep it for up to 24 hours while the browser is offline. When the link would make the message larger than one encrypted record holds, the link becomes `/`.

| Outcome | gnotifd |
|---|---|
| the push service answers 2xx | done; deletes the push's outbox row, if it has one |
| it answers 404 or 410 | deletes the subscription, with its opt-ins and outbox pushes |
| it answers 429 or 5xx, or the request fails on the network | writes the push to the outbox, and retries it from there after 30 seconds, doubling up to one hour, or after the push service's `Retry-After` when that is later |
| it answers any other status, such as 400 or 403 | logs an error and drops the push; the subscription stays |
| the push cannot be encrypted or signed | logs an error and drops the push |
| the push is still unsent a day after it was created | drops it with a warning |

The HTTP client waits 10 seconds for a push service and follows no redirects.

On stop (SIGTERM or Ctrl-C), each push in flight finishes and its outcome is recorded. The sender then writes the pushes still waiting in memory to the outbox, and gnotifd sends them on the next start. A hard kill loses the pushes in memory; the outbox keeps the rest.

A push sent from the outbox, or sent to the outbox at the memory cap, can arrive after a later push to the same subscription.

## Two channels

A match reaches the browsers in two ways. Push shows an OS notification, even with the dapp's tab closed, and needs the browser's permission and a service worker. A live connection needs neither: an open page sends its opt-ins over a WebSocket and receives the same title, body and link, with the event's attributes and the block, block time and transaction it came from ([HTTP API](http-api.md#live-notifications)). A dapp uses either, or both with the same opt-ins. A server offers live connections only when its operator turns them on ([Running gnotifd](running-gnotifd.md#flags)).

The watch loop hands every match to the live connections once the database call that committed it succeeds. A live connection hears of a match at most once, and never of one whose call failed.

A notification is a hint, and the chain holds the truth: a page reads the state it shows from the chain, and reads it again after its live connection reconnects.

## The service worker

The dapp's `sw.js` shows each push, opens its link on a click, posts each full push to the open pages in its scope, and [hands a renewed subscription to the server](http-api.md#replace-a-rotated-subscription). [The client's README](../js/README.md#notifications) describes how it behaves.

## Rules for gnotif servers

Any server built on the registry, gnotifd or another, must:

- apply `TriggerDeclared` and `TriggerRemoved` only from events whose package path is a registry version they follow, and only when the id carries that version's prefix;
- match a trigger's target and event type exactly, and skip an event that lacks the trigger's param attribute;
- percent-escape every attribute value it fills into a link, so a value cannot add a path segment or a host;
- fall back to `/` when a filled-in link starts with `//`;
- send a live connection only the matches for its exact opt-ins;
- give a live notification the same title, body and link as the push of the same match;
- give a live notification an `id`, and the full push of the same match the same `id`;
- leave the final origin check to the client: the service worker opens a link only when it resolves to the dapp's own origin.

## Rules for a new registry version

The registry is immutable, so a change ships as a new version at `…/gnotif/v<N+1>`. gnotifd follows every later version, so a new version must keep these rules:

1. **Path.** Deploy it at `…/gnotif/v<N+1>`, in the same namespace.
2. **Not private.** Never set `private = true`. The creator of a private package can redeploy it in place. `init` then runs again with fresh globals, the id counter restarts, and the version reissues its own ids over the stored triggers.
3. **Ids** take the form `v<N+1>-…`.
4. **Existing attributes are frozen.** Never remove, rename or change the meaning of a `TriggerDeclared` or `TriggerRemoved` attribute. gnotifd skips a `TriggerDeclared` that lacks one of the ten.
5. **Attribute hygiene.** Never emit the same key twice, since gnotifd reads the first. Stay within the chain's caps for one event: 64 attribute pairs, and at most 4096 bytes for each type, key and value. v0 uses 10 pairs.
6. **New attributes** must be safe to ignore: presentation only, such as an icon.
7. **Narrowing changes go in a new event type.** Anything that narrows matching, such as an extra filter or an expiry, goes in a new event type, which older servers skip. A new event type may only add triggers. Removal stays `TriggerRemoved`.
8. **Emit from the realm package itself.** Call `chain.Emit` directly in the realm. An emit moved into a `/p/` helper carries the helper's path, and servers miss it.
9. **No other emitters.** Never call a function or interface value the caller supplies, and never export a method or closure that emits registry events outside `Declare` and `Remove`. Registry code emits as the registry, whoever runs it.
10. **`verified` keeps its meaning:** the target realm presented its own live `cur` to `Declare`. `target` never contains `#`, so a sub-realm token (`cur.Sub(x)`, reported as `host#x`) cannot verify.

gnotifd trusts every later version under the prefix and takes `verified` at face value. That trust rests on the chain's deploy gate, which lets only a namespace's owner deploy under it. GovDAO controls that gate, and the operators of a hardfork are a trust root too. An address namespace, `gno.land/r/<address>/gnotif/v<N>`, removes only the risk of a name changing hands; the GovDAO and hardfork risks remain.
