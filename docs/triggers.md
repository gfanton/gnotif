# Triggers

A trigger tells gnotif servers which event of a realm to watch, who hears about it, and what the notification says. Triggers live in the gnotif registry realm, `gno.land/r/nym-gfanton001/gnotif/v1`, the one gnotif.xyz reads.

## Declare your triggers

A realm declares all its triggers in one call, from its `init` function:

```go
import gnotif "gno.land/r/nym-gfanton001/gnotif/v1"

func init(cur realm) {
	me := cur.Sub(gnotif.Subpath) // a variable: cross() takes a bare identifier
	gnotif.Declare(cross(me),
		gnotif.Trigger{Event: "Message", Param: "to", Title: "New message", Body: "{text}", Link: "/"},
		gnotif.Trigger{Event: "Reply", Title: "New reply", Link: "/"},
	)
}
```

`me` is the realm's gnotif identity, which the chain reports as `<realm path>#gnotif`. `cur.Sub` makes it only from the realm's own live `cur`, in code that runs as the realm, and the registry takes a realm's triggers only from that realm's gnotif identity. Every trigger the registry holds carries the [verified mark](how-it-works.md#what-the-verified-mark-means). A realm whose path is longer than 249 bytes cannot make `me`, so keep your realm's path within 249 bytes.

The registry's functions:

```go
func Declare(cur realm, ts ...Trigger) []string
func Drop(cur realm, target, ids string)
func Active(target string) string
```

`Declare` returns the ids of the triggers in its list, in order. `Active` returns a realm's ids, comma-separated, oldest first. Ids count up from `0000001` in v0, and begin with `v<N>-` in version N of the registry, such as `v1-0000001`.

`gnotif.Trigger` holds the six fields a dapp sets ([Fields](#fields)). It is the struct of the package `gno.land/p/nym-gfanton001/gnotif/trigger/v0`, and a dapp imports only the registry to use it.

## Declare once, at deploy

A realm's namespace is the first element of its path after `gno.land/r/`. Its owner is the address it names, as in `gno.land/r/<address>/...`, or the user whose registered name it is. The registry asks `gno.land/r/sys/names` for the owner, as the chain does on every deploy.

The registry takes a realm's triggers in one `Declare` call, and only when the namespace owner signed the transaction with its own key and sent it straight to the realm. The owner's deploy does both, so the triggers are active once the realm goes live. On a chain that does not enforce namespaces, such as a local gnodev, any key's deploy declares them.

After that call, the realm's triggers are set. A trigger identical in every field to one of the realm's triggers, or to an earlier one in the same list, gets that trigger's id. So a later `Declare` that repeats the realm's triggers changes nothing, and an `init` that runs again does no harm. A later `Declare` with any other trigger panics, a dropped one included.

`Declare` panics, which reverts the transaction, when:

| Panic | Cause |
|---|---|
| `call with this realm's gnotif identity (me := cur.Sub(gnotif.Subpath), then cross(me)); got <path>` | the caller is not a realm's gnotif identity: a realm's plain `cur`, or another `Sub` of it |
| `declare at least one trigger` | the list is empty |
| `invalid event`, `invalid filter`, `invalid param`, `invalid title`, `invalid body`, `invalid link` | the first field that breaks its [rule](#fields) |
| `the triggers of <target> are set: deploy a new version of the realm to change them` | the realm declared its triggers before, and the list holds a new one |
| `signed with a session key: sign with the namespace owner's own key` | a session key signed the transaction |
| `<addr> is not authorized for namespace <ns>` | the signer does not own the realm's namespace |
| `the dapp was not entered directly by its namespace owner; entered from <path>` | the signer called another realm, which called the dapp |
| `too many triggers for target` | the list passes the [limit](#at-most-64-triggers-per-realm) |

## At most 64 triggers per realm

A realm declares at most 64 triggers in each registry version.

## One trigger for every user or group

A realm declares its triggers once, so a trigger never names one user, group or game. One trigger with a `param` covers them all: the event names the user or group in that attribute, and each browser opts in with the value it follows, such as its own address or a group's id.

## A trigger's id can change

A trigger cannot be edited, and a realm declares its triggers once. To change them, deploy a new version of the realm at a new path, such as `…/v2`, whose `init` declares the new list. Its triggers get new ids, and opt-ins never move from one trigger to another. The namespace owner [drops](#drop-a-trigger) the old version's triggers when they should stop.

So a page never keeps a trigger id. It asks for the ids with `triggers(target)` each time it loads, and sends its opt-ins with the ids it gets.

## Drop a trigger

`Drop` removes triggers for good. The realm's namespace owner calls it directly, signed with its own key, such as with gnokey:

```sh
gnokey maketx call -pkgpath gno.land/r/nym-gfanton001/gnotif/v1 -func Drop \
  -args gno.land/r/<you>/notify -args v1-0000007 \
  -gas-wanted 20000000 -gas-fee 40000ugnot \
  -chainid onyx-1 -remote https://rpc.onyx.testnets.gno.land:443 mykey
```

The first argument is the realm's path, and the second lists the ids to drop, separated by commas, such as `v1-0000007,v1-0000008`. The realm's [page on gnoweb](#see-the-triggers-on-gnoweb) lists its ids.

Dropping a trigger deletes its opt-ins on every gnotif server. The realm's triggers stay set, even once all of them are dropped: only a new version of the realm can declare new ones.

`Drop` drops nothing and panics when:

| Panic | Cause |
|---|---|
| `only the namespace owner may do this, in a call signed with its own key; got <path>` | a realm called it |
| `sign with the namespace owner's own key, not a session key` | a session key signed the transaction |
| `invalid target <target>` | the first argument is not a realm path |
| `<addr> is not authorized for namespace <ns>` | the signer does not own the realm's namespace |
| `<id> is not an active trigger of <target>` | an id is not one of the realm's triggers, or appears twice |

## Let other code declare your triggers

Code that holds `me` can declare the realm's triggers. To have a library declare them, pass `me` from `init` to a plain function of the library, one whose first parameter is not a `realm`:

```go
// the library's realm
func Setup(_ int, me realm) {
	gnotif.Declare(cross(me), gnotif.Trigger{Event: "Message", Param: "to", Title: "New message", Body: "{text}", Link: "/"})
}

// your realm
func init(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	library.Setup(0, me)
}
```

A crossing function cannot use `me`: it runs with its own `cur`, so `cross(me)` panics there.

## Keep your gnotif identity to your realm

Code that can make your realm's `me` can declare triggers in your realm's name until your realm's triggers are set. Declare in `init`, so that this ends with your deploy. These rules keep other code from making `me`:

- Never pass your `cur` to code you do not control, `/p/` code above all: a `/p/` function given your `cur` runs as your realm and can make `me`.
- Never expose a pointer to your own `/p/` object whose methods call a function they are given.
- Make `me` only from your own crossing function's `cur`. Never write a function that calls `Sub` on a realm value it receives.

## Fields

A dapp sets the first six fields, capitalized in `gnotif.Trigger`: `Event`, `Filter`, `Param`, `Title`, `Body` and `Link`. The registry sets the others.

| Field | Meaning | Rule |
|---|---|---|
| `event` | event type to match | 1 to 64 bytes of `A-Z a-z 0-9 _` |
| `filter` | fixed `key=value` pairs the event must carry, comma-separated; empty for none | up to 8 pairs; each key 1 to 64 bytes of `A-Z a-z 0-9 _`; each value 1 to 64 bytes without `=` or `,` |
| `param` | attribute whose value a browser opts in with; empty notifies every browser that opted in | empty, or 1 to 64 bytes of `A-Z a-z 0-9 _` |
| `title` | notification title template | 1 to 64 bytes |
| `body` | notification body template | at most 255 bytes |
| `link` | path template opened on click, on the dapp's own origin | a path of at most 256 bytes, starting with a single `/`; no ASCII control characters or backslash |
| `target` | package path of the realm whose events the trigger watches: the realm whose gnotif identity declared it | set by the registry |
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

- gnotif reads only the events of successful transactions.
- When an attribute key repeats in one event, gnotif uses its first value.

## Who an event can notify

Anyone can call a realm's functions with any address. When a watched event names an address, any caller can make that address's browsers show a notification. In a dapp, name an address in a watched event only once that address has acted: accepted an invite, joined a game or placed a bid.

gnotif does not check that an address belongs to the browser that opts in with it: anyone can opt in with any address. A notification says only what the event already published on chain.

## See the triggers on gnoweb

Each registry version renders on gnoweb, under its own path:

- `/r/nym-gfanton001/gnotif/v1` lists every trigger, newest first, 50 to a page, and `/r/nym-gfanton001/gnotif/v1:page/2` is the second page;
- `/r/nym-gfanton001/gnotif/v1:trigger/<id>` shows one trigger with its templates;
- `/r/nym-gfanton001/gnotif/v1:target/<realm path>` lists one realm's triggers, such as `:target/gno.land/r/<you>/notify`.
