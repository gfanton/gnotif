# Triggers

A trigger tells gnotif servers which event of a realm to watch, who hears about it, and what the notification says. Triggers live in the gnotif registry realm, `gno.land/r/<namespace>/gnotif/v0`, the one gnotif.xyz reads.

## Declare a trigger

The registry has two functions:

```go
func Declare(cur realm, target, event, filter, param, title, body, link string) string
func Remove(cur realm, id string)
```

`Declare` stores a trigger and returns its id. Ids count up from `0000001`. `Remove` deletes a trigger, and only the address that declared it may call it.

A realm declares its triggers in its `init` function:

```go
func init(cur realm) {
	gnotif.Declare(cross(cur), cur.PkgPath(), "Message", "", "to", "New message", "{msg}", "/")
}
```

- `init` runs once, when the realm is deployed, so nobody else can run it.
- `cross(cur)` makes the realm the caller that the registry sees, so the trigger is verified.
- `cur.PkgPath()` is the realm's own path, whatever path it is deployed at.
- The trigger's declarer is the realm's address, so only the realm can remove it, through a function that calls `gnotif.Remove(cross(cur), id)`.

## The verified mark

A trigger is verified when the caller of `Declare` is the realm at its target path. The mark means "declared by the realm at the target path". It holds because only the owner of a namespace can deploy under it ([How gnotif works](how-it-works.md#what-the-verified-mark-means)).

Anyone can also call `Declare` directly, as a transaction, with any target. The registry stores such a trigger without the mark, and its declarer pays its storage deposit. gnotifd ignores unverified triggers: it never stores, lists or notifies for one.

## At most 64 triggers per realm

A realm holds at most 64 verified triggers. The 65th `Declare` panics with `too many triggers for target`, which reverts the transaction, so a deploy that declares it fails. `Remove` frees a place. Unverified triggers do not count.

gnotifd relies on this limit: it lists at most 64 triggers for one realm.

## A trigger's id can change

A trigger cannot be edited. To change one, a realm removes it and declares a new one. A realm deployed at a new path, such as a `v1`, declares its own new triggers. Each `Declare` makes a new trigger with a new id. Opt-ins never move from one trigger to another, and removing a trigger drops its opt-ins.

So a page never keeps a trigger id. It asks for the ids with `triggers(target)` each time it loads, and sends its opt-ins with the ids it gets.

## Fields

| Field | Meaning | Rule |
|---|---|---|
| `target` | package path of the realm whose events the trigger watches | starts with `gno.land/r/`, followed by `/`-separated segments that each start with `a-z`, continue with `a-z 0-9`, and have `_` or `-` only between letters or digits; at most 256 bytes |
| `event` | event type to match | 1 to 64 bytes of `A-Z a-z 0-9 _` |
| `filter` | fixed `key=value` pairs the event must carry, comma-separated; empty for none | up to 8 pairs; each key 1 to 64 bytes of `A-Z a-z 0-9 _`; each value 1 to 64 bytes without `=` or `,` |
| `param` | attribute whose value a browser opts in with; empty notifies every browser that opted in | empty, or 1 to 64 bytes of `A-Z a-z 0-9 _` |
| `title` | notification title template | 1 to 64 bytes |
| `body` | notification body template | at most 255 bytes |
| `link` | path template opened on click, on the dapp's own origin | 1 to 256 bytes; starts with `/`; does not start with `//`; no byte below `0x20`, no `0x7F` and no backslash |

`Declare` panics on the first field that breaks its rule, which reverts the transaction.

Names are limited to identifier characters, and the target to the characters of a realm path, so that a declarer cannot make a rendered trigger imitate the verified mark. The link is a path because anyone can declare a trigger on any realm: a full URL would let a stranger send a dapp's users to another site. A path always resolves against the origin of the dapp whose page subscribed the browser.

## Templates

In `title`, `body` and `link`, `{key}` stands for the value of the event's attribute `key`.

- A missing attribute renders as an empty string, and a `{` without a closing `}` is copied as is.
- In `link`, every value is escaped as one path segment, with Go's `url.PathEscape`: it cannot carry `/`, `?` or `#`, so it cannot add a path segment or a host. It can still carry `&`, `=` and `+`, so in a query string a value can add a parameter. `/?msg={msg}` renders as `/?msg=Hello%20from%20onyx` when `msg` is `Hello from onyx`.
- gnotifd cuts a rendered title to 64 bytes and a rendered body to 255 bytes, at a character boundary. A rendered link longer than 1,024 bytes, or one that starts with `//` because a value was empty, becomes `/`.

## Matching

An event matches a trigger when:

1. its package path equals `target`, and its type equals `event`;
2. it carries every `filter` pair with the same value;
3. when `param` is set, its attribute `param` equals the value the browser opted in with. An event without that attribute notifies no one.

When `param` is empty, every matching event notifies every browser that opted in to the trigger.

- A trigger names attribute keys of 1 to 64 bytes of `A-Z a-z 0-9 _`, so emit the attributes it reads under such keys.
- gnotif reads only the events of successful transactions.
- When an attribute key repeats in one event, gnotif uses its first value.

## Who an event can notify

Anyone can call a realm's functions with any address. When a watched event names an address, any caller can make that address's browsers show a notification. In a dapp, name an address in a watched event only once that address has acted: accepted an invite, joined a game or placed a bid.

gnotif does not check that an address belongs to the browser that opts in with it: anyone can opt in with any address. A notification says only what the event already published on chain.

## See the triggers on gnoweb

The registry renders on gnoweb, under its path:

- `/r/<namespace>/gnotif/v0` lists every trigger, newest first, 50 to a page, and `/r/<namespace>/gnotif/v0:page/2` is the second page;
- `/r/<namespace>/gnotif/v0:trigger/<id>` shows one trigger with its templates;
- `/r/<namespace>/gnotif/v0:target/<realm path>` lists one realm's verified triggers, such as `:target/gno.land/r/<you>/notify`.
