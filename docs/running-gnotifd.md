# Running gnotifd

gnotifd is one binary that keeps its state in one SQLite file, or in a [remote store](#remote-store). It needs a tx-indexer for the chain, the package path of the gnotif registry, and a VAPID key pair to sign its pushes. It serves only the HTTP API under `/v1/` at its root ([HTTP API](http-api.md)); to serve it under a path, such as `/onyx/`, strip that path in front of gnotifd. Dapps serve their own pages.

## Install

With Go 1.27 or later:

```sh
go install github.com/gfanton/gnotif/cmd/gnotifd@latest
```

Or run the [container image](#run-the-container-image).

## Make the VAPID keys

gnotifd signs every push with a VAPID key pair. Make the pair once, in a file that only you can read:

```sh
touch vapid.env
chmod 600 vapid.env
gnotifd keygen > vapid.env
```

`>` writes into the file that already exists and keeps its mode, so only you can read the private key.

The file holds two lines, the environment variables gnotifd reads the pair from:

```sh
GNOTIF_VAPID_PUBLIC_KEY=...
GNOTIF_VAPID_PRIVATE_KEY=...
```

Load them into the terminal you start gnotifd from:

```sh
export $(cat vapid.env)
```

Keep the pair for the life of the server, keep the private key secret, and keep `vapid.env` outside any source checkout. A push service accepts a push only when it is signed with the key its subscription was made with, so a new pair stops every push until each page calls `enable()` again.

## Start gnotifd

```sh
gnotifd -indexer https://indexer.onyx.testnets.gno.land/graphql/query \
  -registry gno.land/r/nym-gfanton001/gnotif/v0 -start-height 256562 \
  -vapid-subject <contact>
```

`<contact>` is an email address or https URL where push services can reach you, such as `ops@example.org`.

gnotifd listens on `127.0.0.1:8080` and writes `gnotif.db` in the current directory. It logs to standard error. Stopping it with Ctrl-C or SIGTERM writes the pushes it still holds in memory to the database before it exits; a hard kill loses them ([How gnotif works](how-it-works.md#delivery-outcomes)).

Dapp pages call gnotifd from their own origins, and a page served over https cannot call a plain http URL on another host, so put gnotifd behind a reverse proxy that terminates TLS.

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `-indexer` | required | tx-indexer GraphQL URL |
| `-registry` | required | package path of the gnotif registry realm, ending in `/v<N>` |
| `-vapid-subject` | required | email address or https URL where push services can reach the operator |
| `-start-height` | none | height to read from on the first start, at or before the registry's deploy |
| `-listen` | `127.0.0.1:8080` | HTTP listen address |
| `-db` | `gnotif.db` | SQLite database file, or the `http://` or `https://` URL of a [remote store](#remote-store) |
| `-poll` | `5s` | indexer poll interval; must be positive |
| `-max-age` | `10m` | oldest event that still sends a notification; must not be negative |
| `-push-hosts` | the four hosts in [the allowlist](#the-push-service-allowlist) | comma-separated push service hosts |
| `-stream-max` | `0` | most live connections held at once; `0` turns live connections off |
| `-hub-listen` | none | address of the [external-hub](#attach-an-external-hub) listener |
| `-publish` | none | URL receiving each window's matches for an [external hub](#attach-an-external-hub) |

`gnotifd -h` prints the flags with their defaults. Durations take Go's syntax, such as `30s`, `10m` or `1h`. The VAPID keys come only from `GNOTIF_VAPID_PUBLIC_KEY` and `GNOTIF_VAPID_PRIVATE_KEY`, the publish token only from `GNOTIF_PUBLISH_TOKEN`, and every other setting only from flags.

## The indexer and the start height

`-indexer` is the GraphQL URL of a [tx-indexer](https://github.com/gnolang/tx-indexer) that reads the chain. onyx's public indexer answers at `https://indexer.onyx.testnets.gno.land/graphql/query`. gnotifd sends it one request per poll, so `-poll` sets the load. While it reads a block too large to read whole, a poll sends one request per transaction in that block.

`-registry` is the path the registry is deployed at. It ends in `/v<N>`, with no leading zero, such as `gno.land/r/nym-gfanton001/gnotif/v0`. gnotifd builds its triggers from that version's events and from every later version under the same prefix, never from an earlier one. A server set to v0 follows v0 and v1. To stop following an older version, start a new database at the later version.

`-start-height` matters only while the database is empty: the first start reads the chain from that height, and gnotifd refuses to start without it. Later starts resume where the database stopped and ignore the flag. Use a height at or before the deploy of the configured version. Registry v0 was deployed on onyx at height 256563, so the examples use 256562. [Deploying the registry](deploying-realms.md#4-deploy-the-registry) notes the indexer's height right before the deploy for this. A height far below the deploy only costs time ([How gnotif works](how-it-works.md#how-gnotifd-reads-the-chain)).

While it catches up, gnotifd skips realm events older than `-max-age`, so a restart after downtime sends no burst of stale notifications. It always applies the registry's events. Against a chain that makes blocks only on transactions, such as a local gnodev, raise `-max-age`, to `1h` for instance: the first block after a quiet spell can carry an old time.

When the indexer reports a height below what gnotifd already stored, gnotifd logs `indexer behind stored bound` on every poll until the indexer catches up, and waits for it. That happens when the indexer re-syncs from scratch, or when several indexers answer behind one URL.

gnotifd skips a transaction it cannot process whole, and logs `skip oversized transaction` as an error, with its height, index and hash. That happens when the indexer's answer for the transaction is too large to read, or, with a [remote store](#remote-store), when committing it takes more than 1,000 statements, about one per event that matches a trigger. Its events are lost, and every other transaction is read. When the lost events declared or removed a trigger, gnotifd's triggers no longer match the registry's.

## The push service allowlist

gnotifd sends a request to every endpoint it stores, so it accepts a subscription only when the endpoint is an https URL on a known push service. The default list:

| Host | Push service |
|---|---|
| `fcm.googleapis.com` | Google |
| `updates.push.services.mozilla.com` | Mozilla |
| `web.push.apple.com` | Apple |
| `*.notify.windows.com` | Microsoft |

An entry `*.suffix` matches any host under `suffix` on the default port. Any other entry must equal the endpoint's host, port included. Case does not matter. `-push-hosts` replaces the whole list, so repeat the defaults you keep.

## Storage and backups

With a file, the database is one SQLite file in WAL mode: while gnotifd runs, `gnotif.db` sits next to `gnotif.db-wal` and `gnotif.db-shm`. Stop gnotifd before copying the database, and copy any `-wal` or `-shm` file left beside it.

The subscriptions and opt-ins exist only in the database. To start a new database, name a new file or an empty remote store with `-db` and give a `-start-height` at or before the registry's deploy; each browser then turns notifications on again from its dapp's page. Start a new database when:

- the database is lost;
- gnotifd's `-registry` changes, to another registry or another version: the old triggers and opt-ins would stay, and an earlier version's removals would stop applying;
- gnotifd refuses the database with `database was made by an older gnotifd`: an older release made it, before a change to the database's layout.

gnotifd also refuses a database made by a newer release, with `database was made by a newer gnotifd`. Run the release that made it instead, since a new database drops every subscription. A refused database is left untouched.

## Remote store

`-db` takes the `http://` or `https://` URL of a remote store in place of a file. The protocol below is the whole contract: any server that speaks it works.

**Request.** gnotifd sends `POST` to the URL, with a JSON array of statements:

```json
[{"sql": "SELECT version FROM gnotifd_schema WHERE id = 1", "args": []},
 {"sql": "DELETE FROM subscriptions WHERE endpoint = ?", "args": ["https://..."]}]
```

`sql` is one SQLite statement, with `?` marks for its `args`. An argument is `null`, a string or a number. gnotifd never sends more than 1,000 statements in one request, so a server need not accept more.

**Answer.** `200` with a JSON array holding one `{"rows": [...], "changes": N}` per statement, in order. `rows` holds one object per row, keyed by column name, and `changes` counts the rows the statement inserted, updated or deleted, and is 0 for a `SELECT` or a schema change. A value is `null`, a string or a number; gnotifd reads a number without a fraction or exponent as an integer.

**Atomic.** The statements of one request run in order as one transaction: all take effect, or none. The server enforces foreign keys, because gnotifd's tables delete their dependent rows with `ON DELETE CASCADE`.

**Errors.** `400` answers a body that is not a JSON array of statements. `500` with `{"error": "<message>"}` answers a failing statement, and the request has no effect. Any other status fails the call, and so does no answer within 40 seconds.

**Access.** The endpoint runs every statement it receives, and gnotifd sends it no credentials. Anyone who can reach it can read, change or drop the subscriptions, keys and opt-ins. Make it reachable only by gnotifd, on a private address or behind network-level access control, and keep its URL private.

**Schema version.** A SQLite file holds the schema version in `PRAGMA user_version`. A remote store holds it in the `gnotifd_schema` table, with one row `id = 1`. gnotifd refuses a store that holds one of its tables but no version, with `database was made by an older gnotifd`. Tables gnotifd does not create do not count.

## Attach an external hub

gnotifd holds live connections itself once `-stream-max` allows them; at its default of 0 it refuses them all. A deployment can instead hold them in an external hub, a separate service, and leave `-stream-max` at 0. gnotifd still builds every frame a client receives, so clients see no difference. Two flags connect the hub, and gnotifd refuses one without the other:

- `-hub-listen <addr>` opens a second listener with one route, `POST /message`. The hub posts each text frame a client sends, at most 8,192 bytes, and receives `{"reply": "<frame>", "keys": ["<key>", "..."]}`. It sends `reply` to the client as is. `keys` is present when the frame replaced the connection's set, and lists that set. Keep this listener off the public network.
- `-publish <url>` posts each processed window's matches to the hub as `[{"key": "<key>", "frame": "<frame>"}]`, in chain order, and the hub sends each `frame` to the connections whose keys include `key`. With `GNOTIF_PUBLISH_TOKEN` set, each request carries `Authorization: Bearer <token>`. A request that fails or takes over 5 seconds is logged and dropped, and so is a batch that arrives while 64 wait.

The hub answers the transport itself, with the texts of the [HTTP API](http-api.md#live-notifications): the exact pong to the exact ping, the closes for an oversized or a binary frame, and the 503 answer past its own cap. From a checkout, `GNOTIF_SERVER=https://<server> GNOTIF_TARGET=<realm path> make conformance` checks a server's live endpoint, with a hub or without, opting into a verified trigger of that realm.

## Run the container image

Build the image from a checkout:

```sh
docker build -t gnotifd .
docker run --rm gnotifd -h
```

A release tag `vX.Y.Z` also publishes `ghcr.io/gfanton/gnotifd:X.Y.Z`, built for linux/amd64.

The image runs a static gnotifd as a non-root user, with `-db /data/gnotif.db -listen 0.0.0.0:8080` by default and a volume at `/data`. Arguments to `docker run` replace those defaults, so pass `-db` and `-listen` again with your flags:

```sh
docker run -d --name gnotifd -p 127.0.0.1:8080:8080 -v gnotif-data:/data --env-file vapid.env \
  gnotifd -db /data/gnotif.db -listen 0.0.0.0:8080 \
  -indexer https://indexer.onyx.testnets.gno.land/graphql/query \
  -registry gno.land/r/nym-gfanton001/gnotif/v0 -start-height 256562 \
  -vapid-subject <contact>
```

`--env-file` reads the `vapid.env` that `gnotifd keygen` wrote.

## What version 0 leaves to the operator

- **Health:** there is no health endpoint. `GET /v1/vapid` answers while the HTTP server runs, but says nothing about the watch loop: watch the log for `watch tick failed`.
- **Abuse limits:** gnotifd has no caps per IP and no send budget per declarer. Anyone can register subscriptions and opt-ins, and a subscription stays until its page deletes it or a push to it comes back expired. `-stream-max` caps live connections in total, not per client. Rate-limit `/v1/` at the reverse proxy.
- **Monitoring:** there are no metrics, only the log. gnotifd logs nothing for a push it sent, nor for a retry after a 429 or 5xx answer. It logs a network failure and a push dropped after a day as warnings, a push the service rejected as an error, and an expired subscription at info level. A live connection refused at `-stream-max`, and one dropped for reading too slowly, are logged as warnings, and a live opt-in set it refuses at info level, with the reason and the number of opt-ins.
