# HTTP API

gnotifd serves its API under `/v1/`. Request and response bodies are JSON. Every `/v1/` answer allows any origin, and `OPTIONS` answers a browser's preflight with 204, so a dapp's page calls the API from its own origin. The API uses no cookies or other credentials. The `gnotif` npm package wraps these calls ([the client's README](../js/README.md)).

| Method and path | Body | Answer |
|---|---|---|
| `GET /v1/vapid` | | 200, the server's VAPID public key |
| `GET /v1/triggers?target=<realm path>` | | 200, the realm's verified triggers |
| `PUT /v1/subscription` | a push subscription, with an optional `oldEndpoint` | 204 |
| `PUT /v1/subscription/optins` | the subscription's endpoint and its opt-ins | 204 |
| `DELETE /v1/subscription` | the subscription's endpoint | 204 |

The API has no accounts. A subscription is known by its push endpoint, which only the browser, its push service and the gnotif server know, and knowing the endpoint is enough to change or delete it.

## Get the VAPID key

`GET /v1/vapid` answers the key a browser subscribes with, as unpadded base64url:

```json
{"publicKey": "BO_JMLuc47778WCmQARvoO7_yLkXrl1fhV92AEkiKv-CMF7L4lglQDzbQqsO-TjsoLeXXQiwcQoebx2v_icTqn0"}
```

## List a realm's triggers

`GET /v1/triggers?target=<realm path>` answers the verified triggers whose target is that realm, in id order. For `target=gno.land/r/dev/pingpong/v0`:

```json
[
  {
    "id": "0000001",
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
  "endpoint": "https://fcm.googleapis.com/fcm/send/...",
  "expirationTime": null,
  "keys": {"p256dh": "...", "auth": "..."}
}
```

It stores the subscription and answers 204. For an endpoint already stored, it replaces the keys and keeps the opt-ins. Fields the server does not use, such as `expirationTime`, are ignored.

The server checks:

- `endpoint` is an https URL of at most 1,000 bytes, without user information, whose host is on the push service allowlist ([Running gnotifd](running-gnotifd.md#the-push-service-allowlist));
- `keys.p256dh` is 65 bytes in base64url and a point on the P-256 curve;
- `keys.auth` is 16 bytes in base64url.

Padding with `=` is accepted on both keys.

### Replace a rotated subscription

A browser may replace its push subscription with a new endpoint. The service worker then sends the new subscription with the old endpoint as `oldEndpoint`:

```json
{
  "endpoint": "https://fcm.googleapis.com/fcm/send/new...",
  "keys": {"p256dh": "...", "auth": "..."},
  "oldEndpoint": "https://fcm.googleapis.com/fcm/send/old..."
}
```

The subscription stored at `oldEndpoint` takes the new endpoint and keys, and keeps its opt-ins and queued pushes. When `oldEndpoint` is not stored, or the new endpoint already is, the request acts as one without `oldEndpoint`.

## Set the opt-ins

`PUT /v1/subscription/optins` replaces the whole set of the subscription's opt-ins, and answers 204:

```json
{
  "endpoint": "https://fcm.googleapis.com/fcm/send/...",
  "optins": [{"trigger": "0000001", "value": "g1..."}]
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
{"endpoint": "https://fcm.googleapis.com/fcm/send/..."}
```

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
