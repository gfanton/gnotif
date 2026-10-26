# Managing triggers

Your realm sets its triggers with a handle the registry gives it once, in a transaction you sign as the owner of its namespace: the address in its path, or your registered name. Keep that handle and your realm can change its triggers after its deploy, or let admins change them. As the namespace owner, you can also take the handle back and drop triggers.

[Triggers](triggers.md) covers what a trigger says, and the [Registry reference](registry.md) lists the registry's functions and panics.

## Open once, in init

The registry gives your realm its handle in an `Open` call, and only when you signed the transaction with your own key and sent it straight to the realm. Your deploy does both, so a realm that opens in `init` holds its handle once it goes live. On a chain that does not enforce namespaces, such as a local gnodev, any key's deploy opens it.

A realm holds one handle at a time: `Open` panics while it holds one. Open in `init`, because a realm that opens from a later function lets other code open first. During one of your direct calls, code that runs as your realm through one of the mistakes in [Keep your gnotif identity to your realm](#keep-your-gnotif-identity-to-your-realm) can take the handle. Your realm's own `Open` then panics until you [revoke](#revoke-a-handle) it. The handle keeps working for that code after the call: whenever any caller reaches such a mistake again, it can change your triggers, until you revoke it.

A realm that keeps no reference to its handle, as in [Set your triggers](triggers.md#set-your-triggers), never changes its triggers.

## Change your triggers later

A realm that keeps its handle can change its triggers at any time. Keep it in an unexported variable, never return it from a function, and mint `me` in the same call that uses it:

```gno
var h *gnotif.Handle

func init(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	h = gnotif.Open(cross(me))
}

func setWelcome(cur realm, title string) {
	me := cur.Sub(gnotif.Subpath)
	h.Set(cross(me), "welcome", gnotif.Trigger{
		Event: "Joined", Param: "who",
		Title: title, Link: "/",
	})
}
```

`setWelcome` is unexported, so only your realm's own code calls it. Anyone can call your realm's exported functions, so an exported function that changes triggers checks its caller first, as [the admins package](#let-admins-change-your-triggers) does.

- `Set` on a new name adds a trigger with a new id.
- `Set` on a name in use, with the same event, filter and param, changes its title, body and link. The id stays, and so does every user's choice to receive it. A `Set` that changes nothing emits nothing.
- To watch another event, filter or param, `Remove` the name, then `Set` it again. The trigger gets a new id, and users turn it on again: a user who turned one trigger on never receives another.
- `Remove` deletes the trigger, and every gnotif server deletes the users' choices to receive it.

## Let admins change your triggers

The package `gno.land/p/nym-gfanton001/gnotif/admins/v0` lets a fixed list of addresses change your realm's triggers. Give it the handle `Open` returned, keep it unexported, and pass `me` on each call:

```gno
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
	return triggers.Set(0, me, "welcome", gnotif.Trigger{
		Event: "Joined", Param: "who",
		Title: title, Link: "/",
	})
}

// Reopen gives the realm a new handle and a new list of admins, after its
// owner revoked the old handle.
func Reopen(cur realm, addrs ...address) {
	me := cur.Sub(gnotif.Subpath)
	triggers = admins.New(gnotif.Open(cross(me)), addrs...)
}
```

Replace each `g1…` with an admin's address: `admins.New` panics on the placeholder. The `0` fills a placeholder first parameter of `triggers.Set`: a function whose first parameter is a `realm` is a crossing function, and a crossing function cannot use a `me` its caller made.

`admins` takes the caller from `me`: whoever called the function that made it, a user or a realm such as a DAO's executor. It refuses anyone else, and a transaction signed with a session key, so a session an admin approves for the realm cannot change its notifications. The [Registry reference](registry.md#the-admins-package) lists its panics.

The list of admins is fixed. To change it, [revoke](#revoke-a-handle) the handle, then call `Reopen` with the new list, each address as its own `-args`. `Open` refuses every caller but your direct call, so `Reopen` is safe to leave public.

## Pause notifications

The registry has no pause. A realm that may need one builds it into its own events: while it emits nothing, gnotif servers send nothing, and its triggers and opt-ins stay. The [Getting started](getting-started.md) realm, with a switch that only its deployer can flip:

```gno
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

## Revoke a handle

`Revoke` ends your realm's handle: `Set` and `Remove` panic on it from then on, and the realm's triggers stay. Call it yourself, signed with your own key:

```sh
gnokey maketx call -pkgpath gno.land/r/nym-gfanton001/gnotif/v1 -func Revoke \
  -args gno.land/r/<you>/notify \
  -gas-wanted 20000000 -gas-fee 40000ugnot \
  -chainid onyx-1 -remote https://rpc.onyx.testnets.gno.land:443 mykey
```

Your realm can then open a new handle, from a function that calls `Open`, in a call you sign and send straight to it. Expose one such function; `Open` refuses every other caller, so it is safe to leave public:

```gno
// Reopen gives the realm a new handle after its owner revoked the old one.
func Reopen(cur realm) {
	me := cur.Sub(gnotif.Subpath)
	h = gnotif.Open(cross(me))
}
```

A realm that opens only in `init` cannot open again. Other code can open a handle in its place when it can make the realm's `me` during one of your direct calls. A new handle takes over the names of the realm's triggers.

To take back a handle other code opened, or one whose admins misbehave, make these three calls and no other call into the realm in between, since only your direct call into it lets other code open first:

1. Revoke the handle.
2. [Drop](#drop-a-trigger) the triggers you do not want.
3. Call your realm's reopen function.

A private realm, one deployed with `private = true`, redeployed in place runs `init` again, and its `Open` panics while the old handle is live. Revoke the handle before you redeploy.

> [!WARNING]
> Renaming or deleting your registered name leaves its namespace with no owner, for good, so nobody can revoke or drop under it again. Revoke and drop what you need under a name before you rename or delete it.

## Drop a trigger

`Drop` deletes triggers against the realm's will. Call it yourself, signed with your own key. Its first argument is the realm's path, and the second lists the ids to drop, separated by commas, such as `v1-0000007,v1-0000008`; spaces around an id are ignored. The realm's [page on gnoweb](triggers.md#see-the-triggers-on-gnoweb) lists its ids.

> [!CAUTION]
> A drop cannot be undone, and it deletes the trigger's opt-ins on every gnotif server. Check each id on the realm's page first, and add `-simulate only` to the command to try it without sending it: an id that is not active fails there.

```sh
gnokey maketx call -pkgpath gno.land/r/nym-gfanton001/gnotif/v1 -func Drop \
  -args gno.land/r/<you>/notify -args v1-0000007 \
  -gas-wanted 20000000 -gas-fee 40000ugnot \
  -chainid onyx-1 -remote https://rpc.onyx.testnets.gno.land:443 mykey
```

`Drop` leaves the realm's handle live, so the realm can set the trigger again. To stop a realm, [revoke](#revoke-a-handle) its handle first, then drop.

## Keep your gnotif identity to your realm

Code that can make your realm's `me` can open its handle until your realm opens it, and code that holds both the handle and `me` can change your triggers. Open in `init`, so that from your deploy on, no other code can open it, and give the handle and `me` only to code you have read, such as a `/p/` package like `admins`. These rules keep other code from making `me` and from using your handle:

- Never pass your `cur` to code you do not control, `/p/` code above all: a `/p/` function given your `cur` runs as your realm and can make `me`.
- Never expose a pointer to your own `/p/` object whose methods call a function they are given.
- Make `me` only from your own crossing function's `cur`, and pass it to a package that never calls `Sub` itself. Never write a function that calls `Sub` on a realm value it receives.
- Never export your handle, or a value that holds it, and never return either from a function. Another realm cannot use them, but code that runs as your realm can.

A crossing function cannot use a `me` that its caller made: it runs with its own `cur`, so `cross(me)` panics there. Make `me` in the function that uses it, or pass it to a plain function, one whose first parameter is not a `realm`.
