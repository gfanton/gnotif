# HTTP API

A dapp's page talks to gnotifd over this API to subscribe the browser, choose its triggers and receive live notifications. The `gnotif` npm package wraps these calls ([the client's README](../js/README.md)), so a page that uses it never calls them itself.

gnotifd serves the API under `/v1/` at its root, and a hosted server can put it under a path: the public onyx server answers at `https://gnotif.xyz/onyx/v1/`. Request and response bodies are JSON. Every `/v1/` answer allows any origin, and `OPTIONS` answers a browser's preflight with 204, so a page calls the API from its own origin. The API uses no cookies or other credentials.

The routes:

| Method and path | Body | Answer |
|---|---|---|
| `GET /v1/vapid` | | 200, the server's VAPID public key |
| `GET /v1/triggers?target=<realm path>` | | 200, the realm's verified triggers |
| `PUT /v1/subscription` | a push subscription, with an optional `oldEndpoint` | 204 |
| `PUT /v1/subscription/optins` | the subscription's endpoint and its opt-ins | 204 |
| `DELETE /v1/subscription` | the subscription's endpoint | 204 |
| `GET /v1/stream` | a WebSocket upgrade | 101, a [live connection](#live-notifications) |

The API has no accounts. A subscription is known by its push endpoint, which only the browser, its push service and the gnotif server know, and knowing the endpoint is enough to change or delete it.

## Get the VAPID key

`GET /v1/vapid` answers the key a browser subscribes with, as unpadded base64url, such as:

```json
{"publicKey": "BO_JMLuc47778WCmQARvoO7_yLkXrl1fhV92AEkiKv-CMF7L4lglQDzbQqsO-TjsoLeXXQiwcQoebx2v_icTqn0"}
```

## List a realm's triggers

`GET /v1/triggers?target=<realm path>` answers the verified triggers whose target is that realm, oldest registry version first and in id order within each, at most 64 from each version. For `target=gno.land/r/dev/pingpong/v0`:

```json
[
  {
    "id": "v1-0000001",
    "target": "gno.land/r/dev/pingpong/v0",
    "event": "TurnPlayed",
    "filter": "",
    "param": "next",
    "title": "Your turn",
    "body": "Game {game}, turn {turn}",
    "link": "/?game={game}",
    "declarer": "g1tw6tyv67zjyf26nskj6g5z9fhtvv4996w50xwf",
    "verified": true
  }
]
```

[Triggers](triggers.md#fields) describes each field. Ids are opaque strings, such as `0000001` or `v1-0000001`.

A realm without verified triggers gets `[]`.

## Register a subscription

`PUT /v1/subscription` takes the browser's push subscription as `PushSubscription.toJSON()` returns it:

```json
{
  "endpoint": "https://fcm.googleapis.com/fcm/send/<id>",
  "expirationTime": null,
  "keys": {"p256dh": "<p256dh>", "auth": "<auth>"}
}
```

It stores the subscription and answers 204. For an endpoint already stored, it replaces the keys and keeps the opt-ins. Fields the server does not use, such as `expirationTime`, are ignored.

The server checks:

- `endpoint` is an https URL of at most 1,000 bytes, without user information, whose host is on the push service allowlist ([Running gnotifd](running-gnotifd.md#the-push-service-allowlist));
- `keys.p256dh` is 65 bytes in base64url and a point on the P-256 curve;
- `keys.auth` is 16 bytes in base64url.

Padding with `=` is accepted on both keys.

## Replace a rotated subscription

A browser may replace its push subscription with a new endpoint. The service worker then sends the new subscription with the old endpoint as `oldEndpoint`:

```json
{
  "endpoint": "https://fcm.googleapis.com/fcm/send/<new id>",
  "keys": {"p256dh": "<p256dh>", "auth": "<auth>"},
  "oldEndpoint": "https://fcm.googleapis.com/fcm/send/<old id>"
}
```

The subscription stored at `oldEndpoint` takes the new endpoint and keys, and keeps its opt-ins and queued pushes. When `oldEndpoint` is not stored, or the new endpoint already is, the request acts as one without `oldEndpoint`.

## Set the opt-ins

`PUT /v1/subscription/optins` replaces the whole set of the subscription's opt-ins, and answers 204:

```json
{
  "endpoint": "https://fcm.googleapis.com/fcm/send/<id>",
  "optins": [{"trigger": "v1-0000001", "value": "<address>"}]
}
```

- `optins` holds at most 50 entries, and `[]` clears the set.
- Each `trigger` is the id of a trigger the server holds. It holds verified triggers only.
- `value` is the trigger's param value: non-empty when the trigger has a `param`, `""` when it has none. It is at most 4,096 bytes. The server refuses leading or trailing white space, since a value that carries it never equals an event attribute.
- The same trigger and value twice count once.

The server has no route that reads opt-ins back. A page keeps the set it last sent.

## Delete a subscription

`DELETE /v1/subscription` removes the subscription with its opt-ins and queued pushes, and answers 204:

```json
{"endpoint": "https://fcm.googleapis.com/fcm/send/<id>"}
```

## Live notifications

`GET /v1/stream` with a WebSocket upgrade opens a live connection. While the page stays open, it receives the notifications its opt-ins name, the same ones a push carries. Frames are JSON text. A connection starts with no opt-ins and receives nothing until a set is accepted.

| Request | Answer |
|---|---|
| without a WebSocket upgrade | 426, `{"error": "websocket upgrade required"}` |
| while the server holds its most connections | 503, `{"error": "too many live connections"}` |
| on a server with live notifications off | 503, `{"error": "live notifications are off on this server"}` |

### Client frames

Each text frame of at most 8,192 bytes gets exactly one reply.

| The client sends | The server replies |
|---|---|
| exactly `{"type":"ping"}` | exactly `{"type":"pong"}` |
| `{"type": "optins", "optins": [{"trigger": "0000001", "value": "<address>"}]}` | `{"type": "optins"}`, or `{"type": "error", "error": "<message>"}` |
| text that is not a JSON object, or whose `type` or `optins` has the wrong type | `{"type": "error", "error": "invalid JSON message"}` |
| a JSON object with another `type`, or none | `{"type": "error", "error": "unknown message type"}` |

The ping is recognised by its exact text: the same JSON spaced differently is an unknown message type. An `optins` frame replaces the connection's whole set. It follows the rules of [Set the opt-ins](#set-the-opt-ins), with the same messages, and a refused set leaves the previous one in place.

### Notification frames

For every match whose trigger and value are in its set, the connection receives:

```json
{"type": "notification", "id": "<id>", "trigger": "0000001", "value": "<address>",
 "title": "Your turn", "body": "Game 0000001, turn 3", "link": "/?game=0000001",
 "height": 181221, "tx": "<tx hash>", "time": "2026-10-16T21:46:08.5Z",
 "attrs": [{"key": "game", "value": "0000001"}, {"key": "next", "value": "<address>"}, {"key": "turn", "value": "3"}]}
```

`id` identifies the match: a push in the [full form](#push-payload) for the same match carries the same `id`, so a client that hears both can keep one.

`title`, `body` and `link` are those of the push for the same match. `attrs` lists the event's attributes in order. `height` and `tx` name the transaction that emitted the event, so a page can check it on chain. `time` is the block's time, in UTC, as RFC 3339.

Frames follow chain order. Each is sent once, after gnotifd has processed its block, and nothing is stored or replayed: a client that was disconnected misses the frames sent meanwhile.

### Limits and closing

gnotifd closes a live connection:

- with code 1009 on a client frame over 8,192 bytes, and with 1003 on a binary frame;
- with 1013 when the connection falls more than 256 frames behind;
- with 1001 when gnotifd stops.

A server may close a connection at any time. The client reconnects and sends its set again.

## Push payload

A Web Push carries one JSON message in a single encrypted record, so it is at most 3,993 bytes. It has one of two forms:

- The full form is the [notification frame](#notification-frames) without `type`: `id`, `trigger`, `value`, `title`, `body`, `link`, `height`, `tx`, `time` and `attrs`. A push uses it when the JSON fits.
- The short form is `{"title": "<title>", "body": "<body>", "link": "<link>"}`. A push uses it when the full form does not fit. A link that would overflow the record becomes `/`.

A push gnotifd sends straight after reading its match has the full form when it fits. A push that comes back from the outbox (retried, held past the memory cap, or written when gnotifd stopped) does not hold its match, so it always has the short form.

The service worker shows `title`, `body` and `link` from either form, and posts only the full form to the page.

## Errors

An error answers JSON with a message:

```json
{"error": "push service evil.example is not allowed"}
```

| Status | When | Messages |
|---|---|---|
| 400 | `GET /v1/triggers` has a missing or empty `target`, or one over 256 bytes | `target is required`, `invalid target` |
| 400 | the body is not JSON, or a field has the wrong type | `invalid JSON body` |
| 400 | a subscription or an opt-in breaks a rule | a message naming the rule |
| 404 | `PUT /v1/subscription/optins` or `DELETE /v1/subscription` names an endpoint the server does not store | `unknown subscription` |
| 413 | the body is over 8,192 bytes | `request body too large` |
| 500 | the server failed, and logged why | `internal error` |
