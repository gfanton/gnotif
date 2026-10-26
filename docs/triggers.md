# Triggers

A trigger tells gnotif servers which event of a realm to watch, who hears about it, and what the notification says. Triggers live in the gnotif registry realm, `gno.land/r/nym-gfanton001/gnotif/v1`, the one gnotif.xyz reads, and each realm sets its own.

## Set your triggers

A realm sets its triggers from its `init` function, each under a name it chooses:

```gno
import gnotif "gno.land/r/nym-gfanton001/gnotif/v1"

func init(cur realm) {
	me := cur.Sub(gnotif.Subpath) // a variable: cross() takes a bare identifier
	h := gnotif.Open(cross(me))
	h.Set(cross(me), "message", gnotif.Trigger{
		Event: "Message", Param: "to",
		Title: "New message", Body: "{text}", Link: "/",
	})
	h.Set(cross(me), "reply", gnotif.Trigger{
		Event: "Reply", Title: "New reply", Link: "/",
	})
}
```

`me` is the realm's [gnotif identity](registry.md#the-gnotif-identity), and `h` is the handle the registry gives the realm, once. [Managing triggers](managing-triggers.md) shows how a realm changes its triggers after its deploy, and [Use cases](use-cases.md) shows triggers for common notifications.

## Fields

A dapp sets a trigger's name, the first argument of `Set`, and six fields, capitalized in `gnotif.Trigger`: `Event`, `Filter`, `Param`, `Title`, `Body` and `Link`. The registry sets the others.

| Field | Meaning | Rule |
|---|---|---|
| `name` | the trigger's name in its realm, shown on gnoweb and sent in `TriggerDeclared`; gnotifd ignores it | 1 to 64 bytes of `A-Z a-z 0-9 _` |
| `event` | event type to match | 1 to 64 bytes of `A-Z a-z 0-9 _` |
| `filter` | fixed `key=value` pairs the event must carry, comma-separated; empty for none | up to 8 pairs; each key 1 to 64 bytes of `A-Z a-z 0-9 _`; each value 1 to 64 bytes without `=` or `,` |
| `param` | attribute whose value a browser opts in with; empty notifies every browser that opted in | empty, or 1 to 64 bytes of `A-Z a-z 0-9 _` |
| `title` | notification title template | 1 to 64 bytes |
| `body` | notification body template | at most 255 bytes |
| `link` | path template opened on click, on the dapp's own origin | a path of at most 256 bytes, starting with a single `/`; no ASCII control characters or backslash |
| `target` | package path of the realm whose events the trigger watches: the realm whose gnotif identity set it | set by the registry |
| `id` | the trigger's id | set by the registry |
| `declarer` | address of the target realm | set by the registry |
| `verified` | always `true` | set by the registry |

The rules keep a trigger from imitating the verified mark or sending a dapp's users to another site ([How gnotif works](how-it-works.md#what-the-verified-mark-means)).

## Templates

In `title`, `body` and `link`, `{key}` stands for the value of the event's attribute `key`.

- A missing attribute renders as an empty string, and a `{` without a closing `}` is copied as is.
- In `link`, every value is escaped as one path segment: it cannot carry `/`, `?` or `#`, so it cannot add a path segment or a host. It can still carry `&`, `=` and `+`, so in a query string a value can add a parameter. `/?msg={msg}` renders as `/?msg=Hello%20from%20onyx` when `msg` is `Hello from onyx`.
- gnotifd cuts a rendered title to 64 bytes and a rendered body to 255 bytes, at a character boundary. A rendered link longer than 1,024 bytes, or one that starts with `//` because a value was empty, becomes `/`.

## Matching

An event matches a trigger when:

1. its package path equals `target`, and its type equals `event`;
2. it carries every `filter` pair with the same value;
3. when `param` is set, its attribute `param` equals the value the browser opted in with. An event without that attribute notifies no one.

When `param` is empty, every matching event notifies every browser that opted in to the trigger.

- The chain marks every event with the path of the realm that emitted it, so no other realm can emit an event in your realm's name.
- gnotif reads only the events of successful transactions.
- When an attribute key repeats in one event, gnotif uses its first value.

## One trigger for every user or group

A realm holds at most 64 triggers in each registry version, and `Set` on a new name past that panics with `too many triggers for target`. So a trigger does not name one user, group or game. One trigger with a `param` covers them all: the event names the user or group in that attribute, and each browser opts in with the value it follows, such as its own address or a group's id.

The event has to carry that attribute, and its value has to name one thing. A reply event without its thread's author cannot notify that author, and a thread number that starts again on every board matches that number on every board.

## Who an event can notify

Anyone can call a realm's functions with any address. When a watched event names an address, any caller can make that address's browsers show a notification. In a dapp, name an address in a watched event only once that address has acted: accepted an invite, joined a game or placed a bid.

gnotif does not check that an address belongs to the browser that opts in with it: anyone can opt in with any address. A notification says only what the event already published on chain.

## A trigger's id can change

A trigger keeps its id while only its title, body or link change. It gets a new id when the realm removes it and sets it again, or when a new version of the realm at a new path, such as `…/v2`, sets it. Opt-ins never move from one trigger to another.

So a page never keeps a trigger id. It asks for the ids with `triggers(target)` each time it loads, and sends its opt-ins with the ids it gets. It picks each trigger by its `event`, `filter` and `param`, never by its place in the list: a trigger set again moves to the end.

## See the triggers on gnoweb

Each registry version renders on gnoweb, under its own path:

- `/r/nym-gfanton001/gnotif/v1` lists every trigger, newest first, 50 to a page, and `/r/nym-gfanton001/gnotif/v1:page/2` is the second page;
- `/r/nym-gfanton001/gnotif/v1:trigger/<id>` shows one trigger with its templates;
- `/r/nym-gfanton001/gnotif/v1:target/<realm path>` lists one realm's triggers, such as `:target/gno.land/r/<you>/notify`.
