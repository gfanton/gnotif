# Protocols

These are the contracts for building a part of gnotif yourself: a gnotif server, a remote store for gnotifd, or a hub for its live connections. Adding notifications to a dapp, or running gnotifd on a SQLite file, needs none of them. A new version of the registry follows the [rules for a new registry version](how-it-works.md#rules-for-a-new-registry-version) instead.

## Registry events

A gnotif server never reads the registry's state. It builds its triggers from the two events the registry emits:

| Event | Attributes |
|---|---|
| `TriggerDeclared` | `id`, `target`, `event`, `filter`, `param`, `title`, `body`, `link`, `declarer`, `verified`, and from v1 `name` and `approver` |
| `TriggerRemoved` | `id` |

`name` is the name the realm set the trigger under. `approver` is the namespace owner who signed the transaction that opened the realm's handle. gnotifd reads only the other ten.

gnotifd applies a registry event only when:

- it comes from the registry version given with `-registry`, or from a later version under the same prefix, such as `…/gnotif/v1` after `…/gnotif/v0`, but never from an earlier one;
- its id carries its own version's prefix: none in v0, whose ids are plain numbers such as `0000001`, and `v<N>-` in version N.

It logs any other event, and any event type it does not know, as `skip registry event` and skips it. Of the events it applies:

- a `TriggerDeclared` stores its trigger only when `verified` is `true`;
- a `TriggerDeclared` for an id gnotifd holds replaces that trigger's title, body and link, and keeps its opt-ins, which is how a registry edits a trigger. One that changes the trigger's target, event, filter or param is logged as `skip registry event` and skipped;
- a `TriggerDeclared` gnotifd cannot read is logged as an error and skipped;
- a `TriggerRemoved` deletes the trigger with its opt-ins, but a push the trigger already queued is still sent.

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

## Remote store

`-db` takes the `http://` or `https://` URL of a remote store in place of a SQLite file. The protocol below is the whole contract: any server that speaks it works.

> [!CAUTION]
> The store runs every statement it receives, and gnotifd sends it no credentials, so anyone who can reach it can read, change or drop the subscriptions, keys and opt-ins. Make it reachable only by gnotifd, on a private address or behind network-level access control, and keep its URL private.

**Request.** gnotifd sends `POST` to the URL, with a JSON array of statements:

```json
[{"sql": "SELECT version FROM gnotifd_schema WHERE id = 1", "args": []},
 {"sql": "DELETE FROM subscriptions WHERE endpoint = ?", "args": ["https://<endpoint>"]}]
```

`sql` is one SQLite statement, with `?` marks for its `args`. An argument is `null`, a string or a number. gnotifd never sends more than 1,000 statements in one request, so a server need not accept more.

**Answer.** `200` with a JSON array holding one `{"rows": [...], "changes": N}` per statement, in order. `rows` holds one object per row, keyed by column name, and `changes` counts the rows the statement inserted, updated or deleted, and is 0 for a `SELECT` or a schema change. A value is `null`, a string or a number; gnotifd reads a number without a fraction or exponent as an integer.

**Atomic.** The statements of one request run in order as one transaction: all take effect, or none. The server enforces foreign keys, because gnotifd's tables delete their dependent rows with `ON DELETE CASCADE`.

**Errors.** `400` answers a body that is not a JSON array of statements. `500` with `{"error": "<message>"}` answers a failing statement, and the request has no effect. Any other status fails the call, and so does no answer within 40 seconds.

**Schema version.** A SQLite file holds the schema version in `PRAGMA user_version`. A remote store holds it in the `gnotifd_schema` table, with one row `id = 1`. gnotifd refuses a store that holds one of its tables but no version, with `database was made by an older gnotifd`. Tables gnotifd does not create do not count.

## External hub

gnotifd holds live connections itself once `-stream-max` allows them; at its default of 0 it refuses them all. A deployment can instead hold them in an external hub, a separate service, and leave `-stream-max` at 0. gnotifd still builds every frame a client receives, so clients see no difference. Two flags connect the hub, and gnotifd refuses one without the other:

- `-hub-listen <addr>` opens a second listener with one route, `POST /message`. The hub posts each text frame a client sends, at most 8,192 bytes, and gets back `{"reply": "<frame>", "keys": ["<key>", "..."]}`. It sends `reply` to the client as is. `keys` is present when the frame replaced the connection's set, and lists that set. Keep this listener off the public network.
- `-publish <url>` posts each processed window's matches to the hub as `[{"key": "<key>", "frame": "<frame>"}]`, in chain order, and the hub sends each `frame` to the connections whose keys include `key`. With `GNOTIF_PUBLISH_TOKEN` set, each request carries `Authorization: Bearer <token>`. A request that fails or takes over 5 seconds is logged and dropped, and so is a batch that arrives while 64 batches wait.

The hub answers the transport itself, with the texts of the [HTTP API](http-api.md#live-notifications): the exact pong to the exact ping, the closes for an oversized or a binary frame, and the 503 answer past its own cap.

From a checkout of this repository, this command checks a server's live endpoint, with a hub or without, opting into a verified trigger of the realm you name:

```sh
GNOTIF_SERVER=https://<server> GNOTIF_TARGET=<realm path> make conformance
```
