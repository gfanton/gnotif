# How gnotif works

gnotifd is one process with one SQLite file. A watch loop reads realm events from tx-indexer and queues a push for every opt-in an event matches. A delivery loop sends the queued pushes. An HTTP server takes the browsers' subscriptions and opt-ins. The two loops share only the queue, kept in the database, so a slow push service never holds back reading the chain, and a crash between them loses nothing.

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

A realm that passes its `cur` to foreign code it calls without crossing lets that code declare triggers in the realm's name. The registry bounds what any trigger can do:

- its names are identifiers and its target holds only the characters of a realm path, so its rendered text cannot imitate the mark;
- its link is a path, which always opens on the dapp's own origin, so a stranger's trigger cannot send the dapp's users to another site;
- the registry's pages sanitize the text a declarer supplied.

## How gnotifd reads the chain

gnotifd polls the indexer every `-poll` interval, 5 seconds by default.

- It reads up to 1,000 blocks at a time, from where it stopped. One request returns every realm event in those blocks, and gnotifd matches the events to its triggers itself.
- After a failure, it reads the same blocks again with a smaller window.
- It never queues the same push twice, so reading a block again sends nothing new.
- It skips a single transaction too large to read, logs an error, and loses only that transaction's events ([Running gnotifd](running-gnotifd.md#the-indexer-and-the-start-height)).

## Storage

One SQLite file holds the triggers, the subscriptions with their opt-ins, and the queued pushes. [Running gnotifd](running-gnotifd.md#storage-and-backups) covers its backups and when to start a new one.

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

The dapp's `sw.js` shows each push, opens its link on a click, and [hands a renewed subscription to the server](http-api.md#replace-a-rotated-subscription). [The client's README](../js/README.md#notifications) describes how it behaves.

## Rules for gnotif servers

Any server built on the registry, gnotifd or another, must:

- apply `TriggerDeclared` and `TriggerRemoved` only from events whose package path is a registry version they follow, and only when the id carries that version's prefix;
- match a trigger's target and event type exactly, and skip an event that lacks the trigger's param attribute;
- percent-escape every attribute value it fills into a link, so a value cannot add a path segment or a host;
- fall back to `/` when a filled-in link starts with `//`;
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
