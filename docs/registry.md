# Registry reference

The gnotif registry is the realm that holds every trigger. gnotif.xyz reads `gno.land/r/nym-gfanton001/gnotif/v1`, and a dapp imports it to set its realm's triggers ([Managing triggers](managing-triggers.md)).

## Functions

A realm sets its triggers through a handle, and a namespace owner revokes a handle or drops triggers directly:

```gno
func Open(cur realm) *Handle
func (h *Handle) Set(cur realm, name string, t Trigger) string
func (h *Handle) Remove(cur realm, name string)
func Drop(cur realm, target, ids string)
func Revoke(cur realm, target string)
func Active(target string) string
```

What each one does:

| Function | What it does |
|---|---|
| `Open` | gives the calling realm its handle, in a transaction its namespace owner signed and sent straight to that realm, such as its deploy |
| `Set` | sets the trigger under a name, and returns its id |
| `Remove` | deletes the trigger under a name |
| `Drop` | deletes triggers by their ids, comma-separated, against the realm's will; called by the namespace owner |
| `Revoke` | ends a realm's handle, and keeps its triggers; called by the namespace owner |
| `Active` | returns a realm's ids, comma-separated, oldest first |

`gnotif.Trigger` holds the six fields a dapp sets ([Fields](triggers.md#fields)). It is the struct of the package `gno.land/p/nym-gfanton001/gnotif/trigger/v0`, and a dapp imports only the registry to use it.

## Trigger ids

Ids count up from `0000001` in v0, the registry's first version, and begin with `v<N>-` in version N, such as `v1-0000001`. Two names can hold identical triggers: they are two triggers, with two ids. [A trigger's id can change](triggers.md#a-triggers-id-can-change) says when a trigger gets a new one.

## The gnotif identity

A realm's gnotif identity, which the chain reports as `<realm path>#gnotif`, is the only caller the registry takes the realm's triggers from. The realm makes it with `me := cur.Sub(gnotif.Subpath)` and calls the registry with `cross(me)`. `cur.Sub` makes it only from the realm's own live `cur`, in code that runs as the realm.

A realm whose path is longer than 249 bytes cannot make `me`, so keep your realm's path within 249 bytes.

## Who owns a namespace

A realm's namespace is the first element of its path after `gno.land/r/`. Its owner is the address it names, as in `gno.land/r/<address>/...`, or the user whose registered name it is. `Open`, `Drop` and `Revoke` each ask `gno.land/r/sys/names` for the owner, as the chain does on every deploy.

- Where namespaces are not enforced, such as on a local gnodev, any key's deploy opens a handle, and nobody can drop or revoke.
- While GovDAO pauses `r/sys/names`, nobody can open, drop or revoke, and live handles keep working.
- A registered name owns its namespace only while it is the user's current name. A rename or a deletion leaves the old namespace with no owner, and the old name cannot be registered again.

## Panics

A panic reverts the whole transaction, so nothing changes.

### Open, Set and Remove

`Open`, `Set` and `Remove` panic when:

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
| `invalid event`, `invalid filter`, `invalid param`, `invalid title`, `invalid body`, `invalid link` | the first field that breaks its [rule](triggers.md#fields) |
| `<name> watches a different event, filter or param: Remove it, then Set it again` | `Set` on a name in use, with another event, filter or param |
| `too many triggers for target` | a new name past the [limit of 64](triggers.md#one-trigger-for-every-user-or-group) |
| `<name> is not a trigger of <target>` | `Remove` of a name that holds no trigger |

### Revoke and Drop

`Revoke` and `Drop` panic when:

| Panic | Cause |
|---|---|
| `only the namespace owner may do this, in a call signed with its own key; got <path>` | a realm called it |
| `sign with the namespace owner's own key, not a session key` | a session key signed the transaction |
| `r/sys/names is not enabled, so no namespace has an owner` | the chain does not enforce namespaces, such as a local gnodev |
| `invalid target <target>` | the target argument is not a realm path |
| `<addr> is not authorized for namespace <ns>` | the signer does not own the realm's namespace |
| `<target> holds no handle` | `Revoke` only: the realm never opened a handle, or it was revoked |
| `<id> is not an active trigger of <target>` | `Drop` only: an id is not one of the realm's triggers, or appears twice |

### The admins package

`gno.land/p/nym-gfanton001/gnotif/admins/v0` ([Let admins change your triggers](managing-triggers.md#let-admins-change-your-triggers)) panics when:

| Panic | Cause |
|---|---|
| `<address> is not an admin` | the caller of the function that made `me` is not on the list |
| `me is not current: mint it in the crossing function the admin called` | `me` was made in an outer call |
| `sign with your own key, not a session key` | a session key signed the transaction |
| `at least one admin` | `admins.New` got an empty list |
| `invalid admin address <address>` | `admins.New` got an address no key can have, such as the placeholder `g1…` |
