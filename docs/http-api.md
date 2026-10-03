# HTTP API

gnotifd serves its API under `/v1/`. Request and response bodies are JSON. Every `/v1/` answer allows any origin, so a dapp's page calls the API from its own origin. The `gnotif` npm package wraps these calls ([the client's README](../js/README.md)).

| Method and path | Body | Answer |
|---|---|---|
| `GET /v1/vapid` | | 200, the server's VAPID public key |
| `GET /v1/triggers` | | 200, every trigger the server knows |
| `PUT /v1/subscription` | a push subscription, with an optional `oldEndpoint` | 204 |
| `PUT /v1/subscription/optins` | the subscription's endpoint and its opt-ins | 204 |
| `DELETE /v1/subscription` | the subscription's endpoint | 204 |

The API has no accounts. A subscription is known by its push endpoint, which only the browser, its push service and the gnotif server know, and knowing the endpoint is enough to change or delete it.

## Get the VAPID key

`GET /v1/vapid` answers the key a browser subscribes with, as unpadded base64url:

```json
{"publicKey": "BO_JMLuc47778WCmQARvoO7_yLkXrl1fhV92AEkiKv-CMF7L4lglQDzbQqsO-TjsoLeXXQiwcQoebx2v_icTqn0"}
```

## List the triggers

`GET /v1/triggers` answers the triggers gnotifd has read from the registry and not seen removed, in id order, or `[]`:

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

`filter` holds comma-separated `key=value` pairs, and `param` is empty for a trigger that takes no value. `title`, `body` and `link` are the templates as declared ([Getting started](getting-started.md#templates)).

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

The subscription stored at `oldEndpoint` takes the new endpoint and keys, and keeps its opt-ins and queued pushes. When `oldEndpoint` is not stored, or the new endpoint already is, the request acts as one without `oldEndpoint`. Knowing an endpoint is the only proof the API asks for, so `oldEndpoint` needs no other.

## Set the opt-ins

`PUT /v1/subscription/optins` replaces the whole set of the subscription's opt-ins, and answers 204:

```json
{
  "endpoint": "https://fcm.googleapis.com/fcm/send/...",
  "optins": [{"trigger": "0000001", "value": "g1..."}]
}
```

- `optins` holds at most 50 entries, and `[]` clears the set.
- Each `trigger` is the id of a trigger the server knows.
- `value` is the trigger's param value: non-empty when the trigger has a `param`, `""` when it has none. It is at most 4,096 bytes, without leading or trailing white space, since such a value never equals an event attribute.
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
| 400 | the body is not JSON, or a field has the wrong type | `invalid JSON body` |
| 400 | a subscription breaks a rule | `endpoint longer than 1000 bytes`, `endpoint must be an https URL`, `endpoint must not carry user information`, `push service <host> is not allowed`, `p256dh must be 65 bytes in base64url`, `auth must be 16 bytes in base64url`, `p256dh must be a P-256 public key` |
| 400 | an opt-in breaks a rule | `more than 50 opt-ins`, `unknown trigger "<id>"`, `trigger <id> takes no value`, `trigger <id> needs a value for <param>`, `value longer than 4096 bytes`, `value has leading or trailing white space` |
| 404 | `PUT /v1/subscription/optins` or `DELETE /v1/subscription` names an endpoint the server does not store | `unknown subscription` |
| 413 | the body is over 8,192 bytes | `request body too large` |
| 500 | the server failed, and logged why | `internal error` |

The opt-ins are checked before the subscription is looked up, so an invalid set answers 400 even for an unknown endpoint.

A method that a `/v1/` path does not take answers 405 in plain text, with an `Allow` header. A path outside `/v1/` answers 404 in plain text.

## CORS

Every `/v1/` answer carries:

```
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, PUT, DELETE, OPTIONS
Access-Control-Allow-Headers: Content-Type
```

`OPTIONS` on any `/v1/` path answers 204, which serves the preflight a browser sends before a JSON `PUT` or `DELETE`. The API uses no cookies or other credentials, so any page may call it.
