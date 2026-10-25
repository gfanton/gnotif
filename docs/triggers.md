# Triggers

A trigger tells gnotif servers which event of a realm to watch, who hears about it, and what the notification says. Triggers live in the gnotif registry realm, `gno.land/r/nym-gfanton001/gnotif/v1`, the one gnotif.xyz reads.

## Set your triggers

A realm opens a handle in the registry from its `init` function, then sets each trigger under a name it chooses:

```go
import gnotif "gno.land/r/nym-gfanton001/gnotif/v1"

func init(cur realm) {
	me := cur.Sub(gnotif.Subpath) // a variable: cross() takes a bare identifier
	h := gnotif.Open(cross(me))
	h.Set(cross(me), "message", gnotif.Trigger{Event: "Message", Param: "to", Title: "New message", Body: "{text}", Link: "/"})
	h.Set(cross(me), "reply", gnotif.Trigger{Event: "Reply", Title: "New reply", Link: "/"})
}
```

`me` is the realm's gnotif identity, which the chain reports as `<realm path>#gnotif`. `cur.Sub` makes it only from the realm's own live `cur`, in code that runs as the realm, and the registry takes a realm's triggers only from that realm's gnotif identity. Every trigger the registry holds carries the [verified mark](how-it-works.md#what-the-verified-mark-means). A realm whose path is longer than 249 bytes cannot make `me`, so keep your realm's path within 249 bytes.

The registry's functions:

```go
func Open(cur realm) *Handle
func (h *Handle) Set(cur realm, name string, t Trigger) string
func (h *Handle) Remove(cur realm, name string)
func Drop(cur realm, target, ids string)
func Revoke(cur realm, target string)
func Active(target string) string
```

`Set` returns the trigger's id. `Active` returns a realm's ids, comma-separated, oldest first. Ids count up from `0000001` in v0, and begin with `v<N>-` in version N of the registry, such as `v1-0000001`.

`gnotif.Trigger` holds the six fields a dapp sets ([Fields](#fields)). It is the struct of the package `gno.land/p/nym-gfanton001/gnotif/trigger/v0`, and a dapp imports only the registry to use it.

## Open once, in init

A realm's namespace is the first element of its path after `gno.land/r/`. Its owner is the address it names, as in `gno.land/r/<address>/...`, or the user whose registered name it is. The registry asks `gno.land/r/sys/names` for the owner, as the chain does on every deploy.

The registry gives a realm its handle in an `Open` call, and only when the namespace owner signed the transaction with its own key and sent it straight to the realm. The owner's deploy does both, so a realm that opens in `init` holds its handle once it goes live. On a chain that does not enforce namespaces, such as a local gnodev, any key's deploy opens it.

A realm holds one handle at a time: `Open` panics while it holds one. Open in `init`. A realm that opens from a later function lets other code open first: code that runs as the realm during one of its owner's direct calls, through one of the mistakes in [Keep your gnotif identity to your realm](#keep-your-gnotif-identity-to-your-realm), can take the handle, and the realm's own `Open` then panics until the owner [revokes](#revoke-a-handle) it. The handle keeps working for that code after the call: whenever any caller reaches a mistake that lets it run as the realm again, it can change the realm's triggers, until the owner revokes it.

A realm that keeps no reference to its handle, as the `init` in [Set your triggers](#set-your-triggers) does, never changes its triggers.

`Open`, `Set` and `Remove` panic, which reverts the transaction, when:

| Panic | Cause |
|---|---|
| `call with this realm's gnotif identity (me := cur.Sub(gnotif.Subpath), then cross(me)); got <path>` | the caller of `Open` is not a realm's gnotif identity: a realm's plain `cur`, or another `Sub` of it |
| `<target> already holds a handle: its namespace owner can Revoke it` | the realm called `Open` while it holds a handle |
| `signed with a session key: sign with the namespace owner's own key` | a session key signed the transaction of `Open` |
| `<addr> is not authorized for namespace <ns>` | the signer of `Open` does not own the realm's namespace |
| `the dapp was not entered directly by its namespace owner; entered from <path>` | the signer called another realm, which called the dapp |
| `not the live handle of <target>` | the handle was revoked, or is a copy |
| `not the gnotif identity of <target>: <path>` | the caller of `Set` or `Remove` is not the realm's gnotif identity |
| `invalid name` | the name is not 1 to 64 bytes of `A-Z a-z 0-9 _` |
| `invalid event`, `invalid filter`, `invalid param`, `invalid title`, `invalid body`, `invalid link` | the first field that breaks its [rule](#fields) |
| `<name> watches a different event, filter or param: Remove it, then Set it again` | `Set` on a name in use, with another event, filter or param |
| `too many triggers for target` | a new name past the [limit](#at-most-64-triggers-per-realm) |
| `<name> is not a trigger of <target>` | `Remove` of a name that holds no trigger |

## Change your triggers later

A realm that keeps its handle can change its triggers at any time. Keep it in an unexported variable, never return it from a function, and mint `me` in the same call that uses it:

```go
var h *gnotif.Handle

func init(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	h = gnotif.Open(cross(me))
}

func setWelcome(cur realm, title string) {
	me := cur.Sub(gnotif.Subpath)
	h.Set(cross(me), "welcome", gnotif.Trigger{Event: "Joined", Param: "who", Title: title, Link: "/"})
}
```

Anyone can call your realm's exported functions, so an exported function that changes triggers checks its caller first, as [the admins package](#let-admins-change-your-triggers) does.

- `Set` on a new name adds a trigger with a new id.
- `Set` on a name in use, with the same event, filter and param, changes its title, body and link. The id stays, and so does every user's choice to receive it. A `Set` that changes nothing emits nothing.
- To watch another event, filter or param, `Remove` the name, then `Set` it again. The trigger gets a new id, and users turn it on again: a user who turned one trigger on never receives another.
- `Remove` deletes the trigger, and every gnotif server deletes the users' choices to receive it.
- Two names can hold identical triggers. They are two triggers, with two ids.

## Let admins change your triggers

The package `gno.land/p/nym-gfanton001/gnotif/admins/v0` lets a fixed list of addresses change a realm's triggers. Give it the handle `Open` returned, keep it unexported, and pass `me` on each call:

```go
import (
	"gno.land/p/nym-gfanton001/gnotif/admins/v0"
	gnotif "gno.land/r/nym-gfanton001/gnotif/v1"
)

var triggers *admins.Admins

func init(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	triggers = admins.New(gnotif.Open(cross(me)), address("g1…"), address("g1…"))
}

// SetWelcome changes the welcome notification's title, when an admin calls it.
func SetWelcome(cur realm, title string) string {
	me := cur.Sub(gnotif.Subpath)
	return triggers.Set(0, me, "welcome", gnotif.Trigger{Event: "Joined", Param: "who", Title: title, Link: "/"})
}

// Reopen gives the realm a new handle and a new list of admins, after its
// owner revoked the old handle.
func Reopen(cur realm, addrs ...address) {
	me := cur.Sub(gnotif.Subpath)
	triggers = admins.New(gnotif.Open(cross(me)), addrs...)
}
```

`admins` takes the caller from `me`: whoever called the function that made it, a user or a realm such as a DAO's executor. It panics with `<address> is not an admin` for anyone else, and with `me is not current: mint it in the crossing function the admin called` for a `me` made in an outer call. It refuses a transaction signed with a session key, with `sign with your own key, not a session key`, so a session an admin approves for the realm cannot change its notifications. `admins.New` panics with `at least one admin` on an empty list, and with `invalid admin address <address>` on an address no key can have, such as this example's `g1…` placeholder.

The list of admins is fixed. To change it, the namespace owner [revokes](#revoke-a-handle) the handle, then calls `Reopen` with the new list, each address as its own `-args`. `Open` refuses every caller but the owner's direct call, so `Reopen` is safe to leave public.

## Revoke a handle

`Revoke` ends a realm's handle: `Set` and `Remove` panic on it from then on, and the realm's triggers stay. The realm's namespace owner calls it directly, signed with its own key, as with [`Drop`](#drop-a-trigger):

```sh
gnokey maketx call -pkgpath gno.land/r/nym-gfanton001/gnotif/v1 -func Revoke \
  -args gno.land/r/<you>/notify \
  -gas-wanted 20000000 -gas-fee 40000ugnot \
  -chainid onyx-1 -remote https://rpc.onyx.testnets.gno.land:443 mykey
```

The realm can then `Open` again, in a call its owner signs and sends straight to it. A realm that wants to recover exposes a function that opens again; `Open` refuses every other caller, so it is safe to leave public:

```go
// Reopen gives the realm a new handle after its owner revoked the old one.
func Reopen(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	h = gnotif.Open(cross(me))
}
```

A realm that opens only in `init` cannot open again. A new handle takes over the names of the realm's triggers.

To take back a handle other code opened, or one whose admins misbehave: revoke it, drop the triggers you do not want, then call the realm's reopen function. Make no other call into the realm in between, since only the owner's direct call into the realm lets other code open first.

A private realm redeployed in place runs `init` again, and its `Open` panics while the old handle is live. Revoke the handle before you redeploy.

`Revoke` panics when:

| Panic | Cause |
|---|---|
| `only the namespace owner may do this, in a call signed with its own key; got <path>` | a realm called it |
| `sign with the namespace owner's own key, not a session key` | a session key signed the transaction |
| `invalid target <target>` | the argument is not a realm path |
| `<addr> is not authorized for namespace <ns>` | the signer does not own the realm's namespace |
| `<target> holds no handle` | the realm never opened a handle, or it was revoked |

## At most 64 triggers per realm

A realm holds at most 64 triggers in each registry version.

## One trigger for every user or group

A realm holds at most 64 triggers, so a trigger does not name one user, group or game. One trigger with a `param` covers them all: the event names the user or group in that attribute, and each browser opts in with the value it follows, such as its own address or a group's id.

The event has to carry that attribute, and its value has to name one thing. A reply event without its thread's author cannot notify that author, and a thread number that starts again on every board matches that number on every board.

## A trigger's id can change

A trigger keeps its id while only its title, body or link change. It gets a new id when the realm removes it and sets it again, or when a new version of the realm at a new path, such as `…/v2`, sets it. Opt-ins never move from one trigger to another.

So a page never keeps a trigger id. It asks for the ids with `triggers(target)` each time it loads, and sends its opt-ins with the ids it gets. It picks each trigger by its `event` and `param`, never by its place in the list: a trigger set again moves to the end.

## Pause notifications

The registry has no pause. A realm that may need one builds it into its own events: while it emits nothing, gnotif servers send nothing, and its triggers and opt-ins stay. Here is the [Getting started](getting-started.md) realm with a switch that only its deployer can flip:

```go
var (
	admin address
	muted bool
)

func init(cur realm) {
	admin = cur.Previous().Address() // the address that deployed the realm
	// ... open the handle and set the triggers ...
}

// SetMuted stops or resumes the realm's notifications.
func SetMuted(cur realm, v bool) {
	if cur.Previous().Address() != admin {
		panic("only the deployer can mute")
	}
	muted = v
}

func Notify(cur realm, to address, msg string) {
	if !muted {
		chain.Emit("Message", "to", to.String(), "msg", msg)
	}
}
```

For a realm deployed without such a switch, stopping its notifications means removing or dropping its triggers, and the opt-ins with them.

## Drop a trigger

`Drop` removes triggers against the realm's will. The realm's namespace owner calls it directly, signed with its own key, such as with gnokey:

```sh
gnokey maketx call -pkgpath gno.land/r/nym-gfanton001/gnotif/v1 -func Drop \
  -args gno.land/r/<you>/notify -args v1-0000007 \
  -gas-wanted 20000000 -gas-fee 40000ugnot \
  -chainid onyx-1 -remote https://rpc.onyx.testnets.gno.land:443 mykey
```

The first argument is the realm's path, and the second lists the ids to drop, separated by commas, such as `v1-0000007,v1-0000008`. Spaces around an id are ignored. The realm's [page on gnoweb](#see-the-triggers-on-gnoweb) lists its ids.

Dropping a trigger deletes its opt-ins on every gnotif server. `Drop` leaves the realm's handle live, so the realm can set the trigger again: to stop a realm, [revoke](#revoke-a-handle) its handle first, then drop.

A drop cannot be undone. Check each id on the realm's page first, and add `-simulate only` to the command to try it without sending it: an id that is not active fails there.

`Drop` drops nothing and panics when:

| Panic | Cause |
|---|---|
| `only the namespace owner may do this, in a call signed with its own key; got <path>` | a realm called it |
| `sign with the namespace owner's own key, not a session key` | a session key signed the transaction |
| `invalid target <target>` | the first argument is not a realm path |
| `<addr> is not authorized for namespace <ns>` | the signer does not own the realm's namespace |
| `<id> is not an active trigger of <target>` | an id is not one of the realm's triggers, or appears twice |

## Let other code change your triggers

Code that holds both your handle and `me` can change your realm's triggers. Give them only to code you have read, such as a `/p/` package like `admins`: make `me` in your own crossing function and pass it to the package, which never calls `Sub` itself.

A crossing function cannot use a `me` that its caller made: it runs with its own `cur`, so `cross(me)` panics there. Make `me` in the function that uses it, or pass it to a plain function, one whose first parameter is not a `realm`.

## Keep your gnotif identity to your realm

Code that can make your realm's `me` can open your realm's handle until your realm opens it. Open in `init`, so that this ends with your deploy. These rules keep other code from making `me` and from using your handle:

- Never pass your `cur` to code you do not control, `/p/` code above all: a `/p/` function given your `cur` runs as your realm and can make `me`.
- Never expose a pointer to your own `/p/` object whose methods call a function they are given.
- Make `me` only from your own crossing function's `cur`. Never write a function that calls `Sub` on a realm value it receives.
- Never export your handle, or a value that holds it, and never return either from a function. Another realm cannot use them, but code that runs as your realm can.

## Fields

A dapp sets a trigger's name, the first argument of `Set`, and six fields, capitalized in `gnotif.Trigger`: `Event`, `Filter`, `Param`, `Title`, `Body` and `Link`. The registry sets the others.

| Field | Meaning | Rule |
|---|---|---|
| `name` | the trigger's name in its realm, shown on gnoweb; not in the registry's events | 1 to 64 bytes of `A-Z a-z 0-9 _` |
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
