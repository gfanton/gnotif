import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const ORIGIN = "https://dapp.example";
const FALLBACK = { title: "New activity", body: "", link: "./" };

const RESUBSCRIBED = { endpoint: "E3", keys: { p256dh: "p3", auth: "a3" } };

// load runs sw.js against fakes, registered with scope. The fake server
// answers putStatus to PUT /v1/subscription and the key "PK" to
// GET /v1/vapid.
function load({ putStatus = 204, windows = [], scope = `${ORIGIN}/`, search = "?server=" + encodeURIComponent("https://gnotif.example") } = {}) {
  const handlers = {};
  const calls = { show: [], open: [], fetch: [], subscribe: [] };
  const self = {
    location: new URL(`${scope}sw.js${search}`),
    registration: {
      scope,
      showNotification: async (...args) => calls.show.push(args),
      pushManager: {
        subscribe: async (options) => {
          calls.subscribe.push(options);
          return { toJSON: () => RESUBSCRIBED };
        },
      },
    },
    clients: { matchAll: async () => windows, openWindow: async (url) => calls.open.push(url) },
    addEventListener: (type, fn) => { handlers[type] = fn; },
  };
  const fetch = async (url, init) => {
    calls.fetch.push({ url, init });
    if (url.endsWith("/v1/vapid")) {
      return { ok: true, status: 200, json: async () => ({ publicKey: "PK" }) };
    }
    return { ok: putStatus < 300, status: putStatus };
  };
  const context = vm.createContext({ self, URL, JSON, console, fetch });
  vm.runInContext(readFileSync(new URL("../src/sw.js", import.meta.url), "utf8"), context);
  return { context, handlers, calls };
}

async function dispatch(handler, event) {
  const pending = [];
  handler({ ...event, waitUntil: (p) => pending.push(p) });
  await Promise.all(pending);
}

test("parsePayload returns the fields of a valid payload", () => {
  const { context } = load();
  const p = context.parsePayload(JSON.stringify({ title: "Your turn", body: "Game 7", link: "/?game=7" }));
  assert.deepEqual({ ...p }, { title: "Your turn", body: "Game 7", link: "/?game=7" });
});

test("parsePayload falls back on malformed payloads", () => {
  const { context } = load();
  for (const text of ["not json", '{"body":"x"}', '{"title":5}']) {
    assert.deepEqual({ ...context.parsePayload(text) }, FALLBACK, text);
  }
});

test("safeLink keeps links on the worker's origin and falls back to its scope", () => {
  const { context } = load();
  for (const scope of [`${ORIGIN}/`, `${ORIGIN}/app/`]) {
    const cases = [
      ["/?game=7", `${ORIGIN}/?game=7`],
      ["/app/?game=7", `${ORIGIN}/app/?game=7`],
      ["./", scope],
      ["//evil.com", scope],
      ["/\\evil.com", scope],
      ["https://evil.com", scope],
      ["javascript:alert(1)", scope],
    ];
    for (const [link, want] of cases) {
      assert.equal(context.safeLink(link, scope), want, `${link} under ${scope}`);
    }
  }
});

test("push with a malformed payload still shows a notification", async () => {
  for (const data of [{ text: () => "nope" }, null]) {
    const { handlers, calls } = load();
    await dispatch(handlers.push, { data });
    assert.equal(calls.show.length, 1);
    assert.equal(calls.show[0][0], "New activity");
  }
});

test("push shows the payload with its link as tag", async () => {
  const { handlers, calls } = load();
  const payload = { title: "Your turn", body: "Game 7", link: "/?game=7" };
  await dispatch(handlers.push, { data: { text: () => JSON.stringify(payload) } });
  const [title, options] = calls.show[0];
  assert.equal(title, "Your turn");
  assert.equal(options.body, "Game 7");
  assert.equal(options.tag, "/?game=7");
  assert.equal(options.renotify, true, "a notification replacing one with the same tag must alert again");
  assert.equal(options.data.link, "/?game=7");
});

test("notificationclick never leaves the worker's origin", async () => {
  const { handlers, calls } = load();
  let closed = false;
  await dispatch(handlers.notificationclick, {
    notification: { data: { link: "//evil.com" }, close: () => { closed = true; } },
  });
  assert.ok(closed);
  assert.deepEqual(calls.open, [`${ORIGIN}/`]);
});

test("notificationclick focuses an open tab before navigating it", async () => {
  const order = [];
  const client = {
    url: `${ORIGIN}/other`,
    focus: async () => { order.push("focus"); },
    navigate: async (url) => { order.push(`navigate ${url}`); },
  };
  const { handlers } = load({ windows: [client] });
  await dispatch(handlers.notificationclick, {
    notification: { data: { link: "/?game=7" }, close: () => {} },
  });
  assert.deepEqual(order, ["focus", `navigate ${ORIGIN}/?game=7`]);
});

test("notificationclick under a path ignores tabs outside the worker's scope", async () => {
  const focused = [];
  const tab = (url) => ({ url, focus: async () => { focused.push(url); }, navigate: async () => {} });
  const { handlers, calls } = load({ scope: `${ORIGIN}/app/`, windows: [tab(`${ORIGIN}/`), tab(`${ORIGIN}/application`)] });
  await dispatch(handlers.notificationclick, {
    notification: { data: { link: "/app/?game=7" }, close: () => {} },
  });
  assert.deepEqual(focused, []);
  assert.deepEqual(calls.open, [`${ORIGIN}/app/?game=7`]);
});

test("notificationclick under a path focuses a tab inside the worker's scope", async () => {
  const order = [];
  const client = {
    url: `${ORIGIN}/app/other`,
    focus: async () => { order.push("focus"); },
    navigate: async (url) => { order.push(`navigate ${url}`); },
  };
  const { handlers } = load({ scope: `${ORIGIN}/app/`, windows: [client] });
  await dispatch(handlers.notificationclick, {
    notification: { data: { link: "/app/?game=7" }, close: () => {} },
  });
  assert.deepEqual(order, ["focus", `navigate ${ORIGIN}/app/?game=7`]);
});

test("a generic notification opens the worker's scope", async () => {
  const { handlers, calls } = load({ scope: `${ORIGIN}/app/` });
  await dispatch(handlers.push, { data: null });
  const [, options] = calls.show[0];
  await dispatch(handlers.notificationclick, { notification: { data: options.data, close: () => {} } });
  assert.deepEqual(calls.open, [`${ORIGIN}/app/`]);
});

test("pushsubscriptionchange registers the new subscription with the server", async () => {
  const { handlers, calls } = load();
  const json = { endpoint: "E2", keys: { p256dh: "p", auth: "a" } };
  await dispatch(handlers.pushsubscriptionchange, { newSubscription: { toJSON: () => json } });
  assert.equal(calls.fetch.length, 1);
  assert.equal(calls.fetch[0].url, "https://gnotif.example/v1/subscription");
  assert.equal(calls.fetch[0].init.method, "PUT");
  assert.deepEqual(JSON.parse(calls.fetch[0].init.body), json);
});

test("pushsubscriptionchange names the old endpoint so the opt-ins move", async () => {
  const { handlers, calls } = load();
  const json = { endpoint: "E2", keys: { p256dh: "p", auth: "a" } };
  await dispatch(handlers.pushsubscriptionchange, {
    oldSubscription: { endpoint: "E1" },
    newSubscription: { toJSON: () => json },
  });
  assert.deepEqual(JSON.parse(calls.fetch[0].init.body), { ...json, oldEndpoint: "E1" });
});

test("pushsubscriptionchange without any subscription subscribes again with the server's key", async () => {
  const { handlers, calls } = load();
  await dispatch(handlers.pushsubscriptionchange, { oldSubscription: null, newSubscription: null });
  assert.deepEqual(calls.subscribe.map((o) => ({ ...o })), [{ userVisibleOnly: true, applicationServerKey: "PK" }]);
  const put = calls.fetch.find((c) => c.init?.method === "PUT");
  assert.deepEqual(JSON.parse(put.init.body), RESUBSCRIBED);
});

test("pushsubscriptionchange fails when the server refuses the new subscription", async () => {
  const { handlers } = load({ putStatus: 400 });
  const json = { endpoint: "E2", keys: { p256dh: "p", auth: "a" } };
  await assert.rejects(dispatch(handlers.pushsubscriptionchange, { newSubscription: { toJSON: () => json } }));
});

test("pushsubscriptionchange fails without a server and makes no request", async () => {
  const { handlers, calls } = load({ search: "" });
  await assert.rejects(dispatch(handlers.pushsubscriptionchange, { newSubscription: null, oldSubscription: null }), /server/);
  assert.equal(calls.fetch.length, 0);
});
