# Running gnotifd

gnotifd is one binary that keeps its state in one SQLite file. It needs a tx-indexer for the chain, the package path of the gnotif registry, and a VAPID key pair to sign its pushes. It serves only the HTTP API under `/v1/` ([HTTP API](http-api.md)). Dapps serve their own pages.

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
  -registry gno.land/r/<namespace>/gnotif/v0 -start-height <height> \
  -vapid-subject <contact>
```

`<contact>` is an email address or https URL where push services can reach you, such as `ops@example.org`.

gnotifd listens on `127.0.0.1:8080` and writes `gnotif.db` in the current directory. It logs to standard error. Stopping it with Ctrl-C or SIGTERM loses no queued push and sends none twice.

Dapp pages call gnotifd from their own origins, and a page served over https cannot call a plain http URL on another host, so put gnotifd behind a reverse proxy that terminates TLS.

## Flags

| Flag | Default | Meaning |
|---|---|---|
| `-indexer` | required | tx-indexer GraphQL URL |
| `-registry` | required | package path of the gnotif registry realm |
| `-vapid-subject` | required | email address or https URL where push services can reach the operator |
| `-start-height` | none | height to read from on the first start, at or before the registry's deploy |
| `-listen` | `127.0.0.1:8080` | HTTP listen address |
| `-db` | `gnotif.db` | SQLite database file |
| `-poll` | `5s` | indexer poll interval; must be positive |
| `-max-age` | `10m` | oldest event that still sends a notification; must not be negative |
| `-push-hosts` | the four hosts in [the allowlist](#the-push-service-allowlist) | comma-separated push service hosts |

`gnotifd -h` prints the flags with their defaults. Durations take Go's syntax, such as `30s`, `10m` or `1h`. The VAPID keys come only from `GNOTIF_VAPID_PUBLIC_KEY` and `GNOTIF_VAPID_PRIVATE_KEY`, and every other setting only from flags.

## The indexer and the start height

`-indexer` is the GraphQL URL of a [tx-indexer](https://github.com/gnolang/tx-indexer) that reads the chain. onyx's public indexer answers at `https://indexer.onyx.testnets.gno.land/graphql/query`. gnotifd sends it one request per poll, so `-poll` sets the load. While it reads a block too large to read whole, a poll sends one request per transaction in that block.

`-registry` is the path the registry is deployed at. gnotifd builds its triggers only from that path's events.

`-start-height` matters only while the database is empty: the first start reads the chain from that height, and gnotifd refuses to start without it. Later starts resume where the database stopped and ignore the flag. Use a height at or before the registry's first event. [Deploying the registry](deploying-realms.md#4-deploy-the-registry) notes the indexer's height right before the deploy for this. A height far below the deploy only costs time ([How gnotif works](how-it-works.md#how-gnotifd-reads-the-chain)).

While it catches up, gnotifd skips realm events older than `-max-age`, so a restart after downtime sends no burst of stale notifications. It always applies the registry's events. Against a chain that makes blocks only on transactions, such as a local gnodev, raise `-max-age`, to `1h` for instance: the first block after a quiet spell can carry an old time.

When the indexer reports a height below what gnotifd already stored, gnotifd logs `indexer behind stored bound` on every poll until the indexer catches up, and waits for it. That happens when the indexer re-syncs from scratch, or when several indexers answer behind one URL.

When the indexer's answer for a single transaction is too large to read, gnotifd skips that transaction and logs `skip oversized transaction` as an error, with its height, index and hash. That transaction's events are lost, and every other transaction is read. When the lost events declared or removed a trigger, gnotifd's triggers no longer match the registry's.

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

The database is one SQLite file in WAL mode: while gnotifd runs, `gnotif.db` sits next to `gnotif.db-wal` and `gnotif.db-shm`. Stop gnotifd before copying the database, and copy any `-wal` or `-shm` file left beside it.

The subscriptions and opt-ins exist only in the database. To start a new database, name a new file with `-db` and give a `-start-height` at or before the registry's deploy; each browser then turns notifications on again from its dapp's page. Start a new database when:

- the database file is lost;
- gnotifd moves to a new registry, which numbers its triggers from `0000001` again, so the old opt-ins would follow the wrong triggers;
- gnotifd refuses the file with `database was made by an older gnotifd`: an older release made it, before a change to the database's layout.

gnotifd also refuses a file made by a newer release, with `database was made by a newer gnotifd`. Run the release that made it instead, since a new database drops every subscription. A refused file is left untouched.

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
  -registry gno.land/r/<namespace>/gnotif/v0 -start-height <height> \
  -vapid-subject <contact>
```

`--env-file` reads the `vapid.env` that `gnotifd keygen` wrote.

## What version 0 leaves to the operator

- **Health:** there is no health endpoint. `GET /v1/vapid` answers while the HTTP server runs, but says nothing about the watch loop: watch the log for `watch tick failed`.
- **Abuse limits:** gnotifd has no caps per IP and no send budget per declarer. Anyone can register subscriptions and opt-ins, and a subscription stays until its page deletes it or a push to it comes back expired. Rate-limit `/v1/` at the reverse proxy.
- **Monitoring:** there are no metrics, only the log. gnotifd logs nothing for a push it sent, nor for a retry after a 429 or 5xx answer. It logs a network failure and a push dropped after a day as warnings, a push the service rejected as an error, and an expired subscription at info level.
