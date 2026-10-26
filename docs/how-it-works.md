# How gnotif works

A notification starts as an event on the gno.land chain and ends in the browser. This page follows it through gnotifd, and gives the rules every gnotif server keeps.

![How a notification travels from a dapp realm to the browser](how-it-works.svg)

1. A dapp realm sets its triggers in the gnotif registry, which emits each one as an event ([Triggers come from the registry's events](#triggers-come-from-the-registrys-events)).
2. tx-indexer indexes the chain. gnotifd polls it for every realm event, and matches each event to the triggers and the browsers' opt-ins ([How gnotifd reads the chain](#how-gnotifd-reads-the-chain)).
3. For each match, gnotifd sends a Web Push through the push service of the browser's vendor ([Delivery outcomes](#delivery-outcomes)).
4. The dapp's service worker, `sw.js`, shows the push as a notification, and opens its link on a click ([The service worker](#the-service-worker)).
5. An open page can also receive each match live, over a WebSocket ([Two channels](#two-channels)).

## Inside gnotifd

gnotifd is one process with one database, a SQLite file or a [remote store](running-gnotifd.md#remote-store). It runs three parts:

- a watch loop, which reads realm events from tx-indexer and hands a push to the sender for every opt-in an event matches;
- the sender, which sends from memory, on one routine per push service;
- an HTTP server, which takes the browsers' subscriptions and opt-ins, and holds the pages' live connections.

The watch loop never waits for a push, so a slow push service never holds back reading the chain. A crash loses the pushes still in memory ([Delivery outcomes](#delivery-outcomes)).

The database holds the triggers, the subscriptions with their opt-ins, the chain position, and the outbox: the pushes waiting for a retry. [Running gnotifd](running-gnotifd.md#storage-and-backups) covers its backups and when to start a new one.

## Triggers come from the registry's events

gnotifd never reads the registry's state. It rebuilds its triggers from the events the registry emits:

| Event | Attributes |
|---|---|
| `TriggerDeclared` | `id`, `target`, `event`, `filter`, `param`, `title`, `body`, `link`, `declarer`, `verified`, and from v1 `name` and `approver` |
| `TriggerRemoved` | `id` |

`name` is the name the realm set the trigger under. `approver` is the namespace owner who signed the transaction that opened the realm's handle. gnotifd reads only the other ten.

gnotifd applies a registry event only when:

- it comes from the registry version given with `-registry`, or from a later version under the same prefix, such as `…/gnotif/v1` after `…/gnotif/v0`, but never from an earlier one ([Rules for a new registry version](deploying-realms.md#rules-for-a-new-registry-version));
- its id carries its own version's prefix: `0000001` in v0, `v<N>-…` in version N.

It logs any other event, and any event type it does not know, as `skip registry event` and skips it. Of the events it applies:

- a `TriggerDeclared` stores its trigger only when `verified` is `true`;
- a `TriggerDeclared` for an id gnotifd holds replaces that trigger's title, body and link, and keeps its opt-ins, which is how a registry edits a trigger. One that changes the trigger's target, event, filter or param is logged as `skip registry event` and skipped;
- a `TriggerDeclared` gnotifd cannot read is logged as an error and skipped;
- a `TriggerRemoved` deletes the trigger with its opt-ins, but a push the trigger already queued is still sent.

The trigger set comes from the chain alone, so any operator can run a server against the same registry and get the same triggers.

[Triggers](triggers.md) covers each field and the [limit of 64 per realm and version](triggers.md#at-most-64-triggers-per-realm).

## What the verified mark means

The verified mark says that the target realm's authority stands behind the trigger. The registry takes a trigger only from the realm's gnotif identity, `<target>#gnotif`, with the handle the registry gave the realm in a transaction that the owner of the realm's namespace signed with its own key and sent straight to the realm, such as the realm's deploy ([Managing triggers](managing-triggers.md#open-once-in-init)). So every trigger it stores carries the mark.

The mark rests on the namespace ownership that the chain checks on every deploy, so it is as strong as that check: on a chain that does not enforce namespaces, whoever deploys a realm can set its triggers.

The mark covers whatever code holds the realm's handle and runs with its gnotif identity: the realm's own code, the code it hands both to, such as the [admins package](managing-triggers.md#let-admins-change-your-triggers), and code that opened the handle before the realm did ([Keep your gnotif identity to your realm](managing-triggers.md#keep-your-gnotif-identity-to-your-realm)).

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

The sender gives each push to the routine of its push service: one per `-push-hosts` entry, and one per host for an endpoint that matches no entry.

- A routine sends its pushes one at a time, in the order it got them, so a slow service delays only its own pushes.
- The pushes go out from memory, and a push that succeeds writes nothing to the database.
- At most 10,000 pushes are held in memory, queued or being sent, across all services. A push past that goes to the outbox.
- The sender reads the outbox at start and every second, takes the pushes whose next attempt is due, oldest first, 100 at a time, and gives them to the routines while memory has room.

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
