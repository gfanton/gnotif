# How gnotif works

gnotifd is one process with one SQLite file. A watch loop reads realm events from tx-indexer and queues a push for every opt-in an event matches. A delivery loop sends the queued pushes. An HTTP server takes the browsers' subscriptions and opt-ins. The two loops share only the queue, kept in the database, so a slow push service never holds back reading the chain, and a crash between them loses nothing.

## Triggers come from the registry's events

gnotifd never reads the registry's state. It rebuilds its triggers from the events the registry emits:

| Event | Attributes |
|---|---|
| `TriggerDeclared` | `id`, `target`, `event`, `filter`, `param`, `title`, `body`, `link`, `declarer`, `verified` (`true` or `false`) |
| `TriggerRemoved` | `id` |

gnotifd applies these events only when they come from the package path given with `-registry`. It stores a trigger only when `verified` is `true`: it skips any other `TriggerDeclared`, so it never lists, matches or notifies for an unverified trigger. It logs a `TriggerDeclared` it cannot read as an error and skips it. Because the trigger set comes from the chain alone, any operator can run a server against the same registry and get the same triggers.

The registry holds at most 64 verified triggers per realm, and refuses the 65th ([Triggers](triggers.md#at-most-64-triggers-per-realm)). So one event matches at most 64 triggers, and gnotifd's list of a realm's triggers holds at most 64.

## What the verified mark means

`Declare` marks a trigger verified when its caller is code whose package path equals the trigger's target. The verified mark means "declared by the realm at the target path".

The mark rests on the chain refusing deploys under a namespace to anyone but its owner. On onyx, a key deploys under its own address, `gno.land/r/<address>/...`, or under a name it registered. On a chain without that rule, whoever deploys a realm at a path earns the mark for that path.

Anyone can declare a trigger without the mark, on any realm. The registry bounds what such a trigger can do: its names are identifiers and its target holds only the characters of a realm path, so its rendered text cannot imitate the mark; its link is a path on the dapp's own origin; and the registry's pages sanitize the text a declarer supplied. gnotifd ignores triggers without the mark.

## The watch window

gnotifd stores a cursor:

- `height_done`, the last height fully processed;
- `bound`, the indexer's latest height as reported on the previous poll;
- `next_tx`, the index of the next transaction to read while gnotifd reads block `height_done` + 1 one transaction at a time, and 0 otherwise.

On the first start the database holds no cursor, so gnotifd needs `-start-height`, 1 or more, and refuses to start without it. It reads the chain from that height on. Later starts resume from the stored cursor and ignore `-start-height`.

A window is bounded by the height the indexer reported on the previous poll, never by the height reported in the same answer. The indexer resolves the fields of one GraphQL query concurrently, so a height reported alongside the events may belong to a block whose transactions the query did not see. A height reported earlier is complete.

Every `-poll` interval, 5 seconds by default, the watch loop runs one tick:

1. When `next_tx` is above 0, it reads the rest of block `height_done` + 1 one transaction at a time, and stops.
2. When `bound` is not above `height_done`, it asks the indexer for its latest height, stores it as `bound`, and stops. When that height is below `height_done`, the indexer is behind, and gnotifd logs a warning.
3. Otherwise the window is the heights above `height_done` up to the smaller of `bound` and `height_done` plus the window size. The window size starts at 1,000 blocks. A failed tick halves it, down to one block, and a successful tick doubles it, up to 1,000.
4. One query asks for every successful transaction in the window that has at least one realm event, whatever the realm, in chain order, and for the times of those blocks.
5. When the indexer reports a latest height below the end of the window, it is behind: it re-synced from scratch, or another instance answers behind the same URL. gnotifd logs a warning, lowers `bound`, and reads the window later.
6. In one SQLite transaction, it walks the events in chain order:
   - a registry event adds or removes a trigger, and a removed trigger takes its opt-ins with it;
   - an event from a block older than `-max-age` is skipped and counted in a warning;
   - any other event is matched against the triggers stored for its realm and type, and every matching opt-in queues one push;
   - `height_done` becomes the end of the window and `bound` the latest height of this answer.
7. When the transaction queued pushes, it wakes the delivery loop.

A tick fails when the indexer is unreachable, answers with a status other than 200, returns a GraphQL `errors` array, or sends an answer too large to read. A failed tick leaves the cursor in place and logs a warning. A SQLite error rolls the whole window back. Either way the next tick starts again from the same height, with a window half as large. A push is queued once per subscription, trigger, transaction and event index, so reading a window twice queues nothing new.

A block is read on the tick after the one that first reports its height, so a notification leaves gnotifd within two poll intervals of the indexer storing its block.

`-max-age`, 10 minutes by default, stops a catch-up after downtime from sending a burst of stale notifications. Registry events are always applied, whatever their age. gnotifd judges age by block time, and a chain that makes blocks only when a transaction arrives, such as a local gnodev, can give the first block after a quiet spell an old time: run gnotifd with a larger `-max-age` against such a chain.

### A block too large to read whole

An indexer answer is too large when it is over 64 MiB, or when the indexer stops with `max elements per query reached`, which it does for a query that matches more than 10,000 transactions. Each too-large answer fails the tick and halves the window, until the window holds one block. When the answer for that one block is still too large, gnotifd reads the block one transaction at a time:

1. It lists the block's successful transactions that have a realm event, by index, with the block's time.
2. It reads each transaction's events, from `next_tx` on. It applies them and stores the next index as `next_tx` in one SQLite transaction, so a restart resumes inside the block.
3. When the answer for one transaction alone is too large, gnotifd skips that transaction and logs `skip oversized transaction` as an error, with its height, index and hash.
4. After the block's last transaction, `height_done` becomes the block's height and `next_tx` returns to 0.

A skipped transaction loses only its own events: the notifications they would have sent, and any trigger they declared or removed. Any other error stops the block at the transaction being read, and the next tick reads that transaction again. gnotifd skips a transaction only when its answer is too large, never on an error that a retry can clear.

## Storage

One SQLite file, set with `-db`, holds everything, in WAL mode with foreign keys on:

| Table | Holds |
|---|---|
| `cursor` | `height_done`, `bound` and `next_tx`, one row |
| `triggers` | every verified trigger the registry declared and has not removed, indexed by target and event |
| `subscriptions` | each browser's push endpoint and its two keys |
| `optins` | which subscription hears which trigger, with its value; removed with the subscription or the trigger |
| `outbox` | queued pushes with their rendered title, body and link, attempt count and next attempt time; removed with the subscription |

The outbox keeps no link to `triggers`: removing a trigger after an event matched it does not cancel the push.

The triggers can be rebuilt from the chain. The subscriptions and opt-ins exist only in this file.

## Delivery outcomes

The delivery loop wakes when the watch loop queues pushes, and every second for retries. It reads the pushes whose next attempt is due, oldest first, 100 at a time, and sends them one by one.

A push carries `{"title", "body", "link"}` as JSON, encrypted for the subscription and signed with the VAPID key. Push services keep it for up to 24 hours while the browser is offline. When the link would make the message larger than one encrypted record holds, the link becomes `/`.

| Outcome | gnotifd |
|---|---|
| the push service answers 2xx | deletes the push |
| it answers 404 or 410 | deletes the subscription, with its opt-ins and queued pushes |
| it answers 429 or 5xx, or the request fails on the network | retries after 30 seconds, doubling up to one hour, or after the push service's `Retry-After` when that is later |
| it answers any other status, such as 400 or 403 | logs an error and drops the push; the subscription stays |
| the push cannot be encrypted or signed | logs an error and drops the push |
| the push is still unsent a day after it was queued | drops it with a warning |

The HTTP client waits 10 seconds for a push service and follows no redirects. On shutdown, the push in flight finishes and its outcome is recorded, so a push the service accepted is not sent again on the next start.

## The service worker

The dapp serves `sw.js` from its own origin and registers it with the gnotif server's URL in its query string.

- **push:** it shows the notification. A notification replaces any earlier one with the same link, and alerts again. A push without a usable payload still shows "New activity", because Safari revokes a subscription whose pushes show nothing.
- **click:** it opens the link on the dapp's origin, and opens the service worker's scope instead when the link lands on another origin. With a tab open under that scope, it focuses the first such tab, and navigates it to the link when the service worker controls that tab; a tab it does not control, such as the one where `enable()` first ran, is only focused. With no tab open under the scope, it opens one at the link.
- **subscription change:** when the browser replaces its push subscription, it sends the new one to the server with the old endpoint as `oldEndpoint`, so the opt-ins carry over ([HTTP API](http-api.md#replace-a-rotated-subscription)).

## Rules for gnotif servers

Any server built on the registry, gnotifd or another, must:

- apply `TriggerDeclared` and `TriggerRemoved` only from events whose package path is the registry's;
- match a trigger's target and event type exactly, and skip an event that lacks the trigger's param attribute;
- percent-escape every attribute value it fills into a link, so a value cannot add a path segment or a host;
- fall back to `/` when a filled-in link starts with `//`;
- leave the final origin check to the client: the service worker opens a link only when it resolves to the dapp's own origin.
