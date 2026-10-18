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
| `TriggerDeclared` | `id`, `target`, `event`, `filter`, `param`, `title`, `body`, `link`, `declarer`, `verified`, and from v1 `approver` |
| `TriggerRemoved` | `id` |

`approver` is the namespace owner who signed the transaction that declared the trigger. gnotifd reads only the other ten.

gnotifd applies these events only when they come from the registry version given with `-registry` or a later version under the same prefix, such as `…/gnotif/v1` after `…/gnotif/v0`. It never reads an earlier version. An event applies only when its id carries its own version's prefix: `0000001` in v0, `v<N>-…` in version N. It logs any other event, and any event type it does not know, as `skip registry event` and skips it. It stores a trigger only when `verified` is `true`. It logs a `TriggerDeclared` it cannot read as an error and skips it. A `TriggerRemoved` deletes the trigger with its opt-ins, but a push the trigger already queued is still sent. Because the trigger set comes from the chain alone, any operator can run a server against the same registry and get the same triggers.

[Triggers](triggers.md) covers each field and the [limit of 64 per realm and version](triggers.md#at-most-64-triggers-per-realm).

## What the verified mark means

The verified mark says that the target realm's authority stands behind the trigger. The registry takes a trigger only from the realm's gnotif identity, `<target>#gnotif`, in a transaction that the owner of the realm's namespace signed with its own key and sent straight to the realm, such as the realm's deploy ([Triggers](triggers.md#declare-once-at-deploy)). So every trigger it stores carries the mark.

The mark rests on the namespace ownership that the chain checks on every deploy, so it is as strong as that check: on a chain that does not enforce namespaces, whoever deploys a realm can declare its triggers.

The mark covers whatever code ran in that transaction with the realm's gnotif identity. A trigger declared by code the realm handed `me` to, or by `/p/` code that ran with the realm's `cur`, carries it too ([Keep your gnotif identity to your realm](triggers.md#keep-your-gnotif-identity-to-your-realm)).

v0's triggers carry the mark on a weaker check: any realm that a dapp passed its `cur` to could declare triggers in the dapp's name. A server that follows v0 keeps that risk.

The registry bounds what any trigger can do:

- its names are identifiers and its target holds only the characters of a realm path, so its rendered text cannot imitate the mark;
- its link is a path, which always opens on the dapp's own origin, so a stranger's trigger cannot send the dapp's users to another site;
- the registry's pages sanitize the text a declarer supplied.

## How gnotifd reads the chain

gnotifd polls the indexer every `-poll` interval, 5 seconds by default.

- It reads up to 1,000 blocks at a time, from where it stopped. One request returns every realm event in those blocks, and gnotifd matches the events to its triggers itself.
- After a failure, it reads the same blocks again with a smaller window.
- It reads the triggers from the database at start, and again after a failed write, and keeps them in memory. Matching an event and checking an opt-in make no database call. A direct edit of the triggers table shows at the next of those reads.
- A window of blocks that holds a registry event or a match is committed in one database call, together with the position. Any other window saves its position every 10 minutes, so a restart can read again what it read in the last 10 minutes.
- It skips a single transaction too large to read, or too large to commit to a remote store, logs an error, and loses only that transaction's events ([Running gnotifd](running-gnotifd.md#the-indexer-and-the-start-height)).

## Storage

The database holds the triggers, the subscriptions with their opt-ins, the chain position, and the outbox: the pushes waiting for a retry. [Running gnotifd](running-gnotifd.md#storage-and-backups) covers its backups and when to start a new one.

## Delivery outcomes

The sender gives each push to the routine of its push service: one per `-push-hosts` entry, and one per host for an endpoint that matches no entry. A routine sends its pushes one at a time, in the order it got them, so a slow service delays only its own pushes. The pushes go out from memory, and a push that succeeds writes nothing to the database. At most 10,000 pushes are held in memory, queued or being sent, across all services. A push past that goes to the outbox.

At start and every second, the sender reads the outbox's pushes whose next attempt is due, oldest first, 100 at a time, and gives them to the routines while memory has room.

A push carries one JSON message, encrypted for the subscription and signed with the VAPID key: the whole notification when it fits one encrypted record, and `{"title", "body", "link"}` otherwise, with `/` as the link when the link would overflow the record ([HTTP API](http-api.md#push-payload)). Push services keep it for up to 24 hours while the browser is offline.

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
5. **Attribute hygiene.** Never emit the same key twice, since gnotifd reads the first. Stay within the chain's caps for one event: 64 attribute pairs, and at most 4096 bytes for each type, key and value. v1 uses 11 pairs.
6. **New attributes** must be safe to ignore: presentation only, such as an icon.
7. **Narrowing changes go in a new event type.** Anything that narrows matching, such as an extra filter or an expiry, goes in a new event type, which older servers skip. A new event type may only add triggers. Removal stays `TriggerRemoved`, and so does anything else that stops notifications, a pause included: an older server skips an event type it does not know, and would keep sending.
8. **Emit from the realm package itself.** Call `chain.Emit` directly in the realm. An emit moved into a `/p/` helper carries the helper's path, and servers miss it.
9. **No other emitters.** Emit registry events only from the registry's own exported functions, and never call a function or interface value a caller supplies.
10. **`verified` means the target realm's authority stands behind the trigger.** In v0 the caller presented the realm's live `cur`. From v1 the realm's gnotif identity, `<target>#gnotif`, declared it in a transaction the target's namespace owner signed and sent straight to the realm. `target` never contains `#`.

gnotifd trusts every later version under the prefix and takes `verified` at face value. That trust rests on the chain's deploy gate, which lets only a namespace's owner deploy under it. GovDAO controls that gate, and the operators of a hardfork are a trust root too. An address namespace, `gno.land/r/<address>/gnotif/v<N>`, removes only the risk of a name changing hands; the GovDAO and hardfork risks remain.
