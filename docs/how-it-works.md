# How gnotif works

A notification starts as an event on the gno.land chain and ends in the browser. On the way, gnotifd, the gnotif server, matches the event to the triggers and sends it as a Web Push, or straight to an open page.

![How a notification travels from a dapp realm to the browser](how-it-works.svg)

The dashed parts ship with the dapp: its realm, its service worker `sw.js`, and its page. The solid ones, the registry, tx-indexer, gnotifd and the push service, are run by gnotif, the chain and the browser's vendor.

1. The dapp's realm calls `Open` and `Set` to set its triggers in the gnotif registry, which emits each change as a `TriggerDeclared` or `TriggerRemoved` event ([Triggers come from the registry's events](#triggers-come-from-the-registrys-events)).
2. The dapp's page subscribes the browser through gnotifd's HTTP API, under `/v1/`, and sends its opt-ins: the triggers it turns on, each with the value it follows, such as an address.
3. tx-indexer indexes the chain and serves it over GraphQL. gnotifd polls it for realm events, the dapp realm's among them, every 5 seconds by default, and matches each event to the triggers and the opt-ins ([How gnotifd reads the chain](#how-gnotifd-reads-the-chain)).
4. For each match, gnotifd sends a Web Push through the push service of the browser's vendor ([Delivery outcomes](#delivery-outcomes)).
5. `sw.js`, served from the dapp's origin, shows the push as an OS notification, and a click on it opens its link on the dapp's page ([The service worker](#the-service-worker)).
6. An open page can also receive each match live, over a WebSocket ([Two channels](#two-channels)).

## Inside gnotifd

gnotifd is one process with one database, a SQLite file or a [remote store](protocols.md#remote-store). It runs three parts:

- a watch loop, which reads realm events from tx-indexer and hands a push to the sender for every opt-in an event matches;
- the sender, which sends from memory, on one routine per push service;
- an HTTP server, which takes the browsers' subscriptions and opt-ins, and holds the pages' live connections.

The watch loop never waits for a push, so a slow push service never holds back reading the chain. A crash loses the pushes still in memory ([Delivery outcomes](#delivery-outcomes)).

The database holds the triggers, the subscriptions with their opt-ins, the chain position, and the outbox: the pushes that wait in the database, for a retry, past the memory cap, or since gnotifd stopped. [Running gnotifd](running-gnotifd.md#storage-and-backups) covers its backups and when to start a new one.

## Triggers come from the registry's events

gnotifd never reads the registry's state. It rebuilds its triggers from the registry's `TriggerDeclared` and `TriggerRemoved` events, from the registry version it is given and every later one under the same prefix. The trigger set comes from the chain alone, so any operator can run a server against the same registry and get the same triggers. [Registry events](protocols.md#registry-events) lists the events' attributes, and when gnotifd applies or skips one.

## What the verified mark means

The verified mark says that the target realm's authority stands behind the trigger. The registry takes a trigger only from the realm's gnotif identity, `<target>#gnotif`, holding the realm's handle. It gives that handle only in a transaction that the owner of the realm's namespace signed with its own key and sent straight to the realm, such as the realm's deploy ([Managing triggers](managing-triggers.md#open-once-in-init)). So every trigger it stores carries the mark.

The mark rests on the namespace ownership that the chain checks on every deploy, so it is as strong as that check:

- on a chain that does not enforce namespaces, whoever deploys a realm can set its triggers;
- GovDAO controls the check, which lets only a namespace's owner deploy under it;
- the operators of a hardfork are a trust root too;
- a registered name can change hands, and an address namespace, `gno.land/r/<address>/...`, removes that risk, but not the other two.

gnotifd trusts every later registry version under its prefix on the same check, and takes its `verified` at face value.

The mark covers whatever code holds the realm's handle and runs with its gnotif identity: the realm's own code, the code it hands both the handle and its gnotif identity to, such as the [admins package](managing-triggers.md#let-admins-change-your-triggers), and code that opened the handle before the realm did, during one of its owner's direct calls ([Keep your gnotif identity to your realm](managing-triggers.md#keep-your-gnotif-identity-to-your-realm)).

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

## Delivery outcomes

The sender gives each push to the routine of its push service: one per entry of the [push service allowlist](running-gnotifd.md#the-push-service-allowlist), and one per host for an endpoint that matches no entry.

- A routine sends its pushes one at a time, in the order it got them, so a slow service delays only its own pushes.
- The pushes go out from memory, and a push that succeeds writes nothing to the database.
- At most 10,000 pushes are held in memory, queued or being sent, across all services. A push past that goes to the outbox.
- The sender reads the outbox at start and every second, takes the pushes whose next attempt is due, oldest first, 100 at a time, and gives them to the routines while memory has room.

A push carries one JSON message, encrypted for the subscription and signed with the VAPID key, in one of the two forms of the [push payload](http-api.md#push-payload). Push services keep it for up to 24 hours while the browser is offline.

What gnotifd does with each answer of the push service:

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

## Rules for a new registry version

The registry is immutable, so a change ships as a new version at `…/gnotif/v<N+1>`. gnotifd follows every later version under its prefix and takes `verified` at face value, so a new version must keep these rules:

1. **Path.** Deploy it at `…/gnotif/v<N+1>`, in the same namespace.
2. **Not private.** Never set `private = true`. The creator of a private package can redeploy it in place. `init` then runs again with fresh globals, the id counter restarts, and the version reissues its own ids over the stored triggers.
3. **Ids** take the form `v<N+1>-…`.
4. **Existing attributes stay frozen.** Never remove, rename or change the meaning of a `TriggerDeclared` or `TriggerRemoved` attribute. gnotifd skips a `TriggerDeclared` that lacks one of the ten attributes it reads ([Registry events](protocols.md#registry-events)). A version may emit `TriggerDeclared` again for one of its own ids to change its title, body or link; it never changes that id's target, event, filter or param, and gnotifd skips a declaration that does.
5. **Attribute hygiene.** Never emit the same key twice, since gnotifd reads the first. Stay within the chain's caps for one event: 64 attribute pairs, and at most 4096 bytes for each type, key and value. v1 uses 12 pairs.
6. **New attributes** must be safe to ignore: presentation only, such as an icon.
7. **New event type for narrowing.** Anything that narrows matching, such as an extra filter or an expiry, goes in a new event type, which older servers skip. A new event type may only add triggers. Removal stays `TriggerRemoved`, and so does anything else that stops notifications, a pause included: an older server skips an event type it does not know, and would keep sending.
8. **Emit from the realm itself.** Call `chain.Emit` directly in the realm. An emit moved into a `/p/` helper carries the helper's path, and servers miss it.
9. **No other emitters.** Emit registry events only from the registry's own exported functions, and never call a function or interface value a caller supplies.
10. **What `verified` means.** `verified` means the target realm's authority stands behind the trigger. In v0 the caller presented the realm's live `cur`. From v1 the realm's gnotif identity, `<target>#gnotif`, set it with the handle the registry gave the realm in a transaction the target's namespace owner signed and sent straight to the realm. `target` never contains `#`.

[What the verified mark means](#what-the-verified-mark-means) says what that trust rests on.
