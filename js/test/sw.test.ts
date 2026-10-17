import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const ORIGIN = "https://dapp.example";

const RESUBSCRIBED = { endpoint: "E3", keys: { p256dh: "p3", auth: "a3" } };

type Shown = [title: string, options: Record<string, unknown>];
type Call = { url: string; init: { method: string; body: string } | undefined };

type WindowSpec = {
  url: string;
  postMessage?: (message: unknown) => void;
  focus?: () => Promise<void>;
  navigate?: (url: string) => Promise<void>;
};

type LoadOptions = {
  putStatus?: number;
  windows?: WindowSpec[];
  matchAllError?: Error | null;
  scope?: string;
  search?: string;
};

type Handler = (event: Record<string, unknown>) => void;

// plain copies a value out of the worker's realm, so deepEqual compares it
// by content.
function plain(value: unknown): unknown {
  return JSON.parse(JSON.stringify(value));
}

function first<T>(items: T[]): T {
  const item = items[0];
  assert.ok(item !== undefined);
  return item;
}

// load runs dist/sw.js against fakes, registered with scope. The fake server
// answers putStatus to PUT /v1/subscription and the key "PK" to
// GET /v1/vapid. Each window records what is posted to it in posted; a
// window's own fields, such as a postMessage that throws, override the defaults.
function load({
  putStatus = 204,
  windows = [],
  matchAllError = null,
  scope = `${ORIGIN}/`,
  search = "?server=" + encodeURIComponent("https://gnotif.example"),
}: LoadOptions = {}) {
  const handlers: Record<string, Handler> = {};
  const calls: {
    show: Shown[];
    open: string[];
    fetch: Call[];
    subscribe: unknown[];
    matchAll: unknown[];
  } = { show: [], open: [], fetch: [], subscribe: [], matchAll: [] };
  const wins = windows.map((w) => {
    const posted: unknown[] = [];
    return { posted, postMessage: (m: unknown) => void posted.push(m), ...w };
  });
  const self = {
    location: new URL(`${scope}sw.js${search}`),
    registration: {
      scope,
      showNotification: async (title: string, options: Record<string, unknown>) => calls.show.push([title, options]),
      pushManager: {
        subscribe: async (options: unknown) => {
          calls.subscribe.push(options);
          return { toJSON: () => RESUBSCRIBED };
        },
      },
    },
    clients: {
      matchAll: async (options: unknown) => {
        calls.matchAll.push(options);
        if (matchAllError !== null) {
          throw matchAllError;
        }
        return wins;
      },
      openWindow: async (url: string) => calls.open.push(url),
    },
    addEventListener: (type: string, fn: Handler) => {
      handlers[type] = fn;
    },
  };
  const fetch = async (url: string, init?: Call["init"]) => {
    calls.fetch.push({ url, init });
    if (url.endsWith("/v1/vapid")) {
      return { ok: true, status: 200, json: async () => ({ publicKey: "PK" }) };
    }
    return { ok: putStatus < 300, status: putStatus };
  };
  const context = vm.createContext({ self, URL, JSON, console, fetch });
  vm.runInContext(readFileSync(new URL("../dist/sw.js", import.meta.url), "utf8"), context);

  async function dispatch(type: string, event: Record<string, unknown>): Promise<void> {
    const handler = handlers[type];
    assert.ok(handler !== undefined, `the worker registered no ${type} handler`);
    const pending: Promise<unknown>[] = [];
    handler({ ...event, waitUntil: (p: Promise<unknown>) => pending.push(p) });
    await Promise.all(pending);
  }

  return { dispatch, calls, wins };
}

const pushed = (text: string | null): Record<string, unknown> => ({
  data: text === null ? null : { text: () => text },
});

test("push with a malformed payload still shows a notification", async () => {
  for (const text of ["nope", null]) {
    const { dispatch, calls } = load();
    await dispatch("push", pushed(text));
    assert.equal(calls.show.length, 1);
    assert.equal(first(calls.show)[0], "New activity");
  }
});

test("push shows the payload with its link as tag", async () => {
  const { dispatch, calls } = load();
  const payload = { title: "Your turn", body: "Game 7", link: "/?game=7" };
  await dispatch("push", pushed(JSON.stringify(payload)));
  const [title, options] = first(calls.show);
  assert.equal(title, "Your turn");
  assert.equal(options["body"], "Game 7");
  assert.equal(options["tag"], "/?game=7");
  assert.equal(options["renotify"], true, "a notification replacing one with the same tag must alert again");
  assert.deepEqual(plain(options["data"]), { link: "/?game=7" });
});

test("push shows the icon the worker was registered with, even on the fallback", async () => {
  const icon = `${ORIGIN}/icon.png`;
  const search = `?server=${encodeURIComponent("https://gnotif.example")}&icon=${encodeURIComponent(icon)}`;
  const payload = JSON.stringify({ title: "Your turn", body: "Game 7", link: "/?game=7" });
  for (const text of [payload, null]) {
    const { dispatch, calls } = load({ search });
    await dispatch("push", pushed(text));
    assert.equal(first(calls.show)[1]["icon"], icon);
  }
});

test("push sets no icon when the worker was registered without one", async () => {
  const { dispatch, calls } = load();
  await dispatch("push", pushed(null));
  assert.equal("icon" in first(calls.show)[1], false);
});

test("a push with an id is shown, then posted to every window in scope", async () => {
  const scope = `${ORIGIN}/app/`;
  const { dispatch, calls, wins } = load({
    scope,
    windows: [{ url: `${ORIGIN}/app/game` }, { url: `${ORIGIN}/other` }],
  });
  const payload = {
    id: "i1",
    trigger: "0000001",
    value: "g1bob",
    title: "Your turn",
    body: "Game 7",
    link: "/app/?game=7",
    height: 7,
    tx: "tx1",
    time: "2026-10-16T21:46:08.5Z",
    attrs: [],
  };
  await dispatch("push", pushed(JSON.stringify(payload)));
  assert.equal(calls.show.length, 1);
  assert.equal(first(calls.show)[0], "Your turn");
  assert.deepEqual(plain(first(calls.matchAll)), { type: "window", includeUncontrolled: true });
  assert.deepEqual(plain(first(wins).posted), [{ type: "gnotif.notification", notification: payload }]);
  assert.deepEqual(wins[1]?.posted, []);
});

test("a short payload is shown and posted nowhere", async () => {
  const { dispatch, calls, wins } = load({ windows: [{ url: `${ORIGIN}/` }] });
  await dispatch("push", pushed(JSON.stringify({ title: "Your turn", body: "Game 7", link: "/?game=7" })));
  assert.equal(calls.show.length, 1);
  assert.deepEqual(first(wins).posted, []);
});

test("a window whose postMessage throws does not stop the others or the notification", async () => {
  const gone = {
    url: `${ORIGIN}/`,
    postMessage() {
      throw new Error("window closed");
    },
  };
  const { dispatch, calls, wins } = load({ windows: [gone, { url: `${ORIGIN}/game` }] });
  const payload = { id: "i1", title: "Your turn", body: "Game 7", link: "/?game=7" };
  await dispatch("push", pushed(JSON.stringify(payload)));
  assert.equal(calls.show.length, 1);
  assert.equal(wins[1]?.posted.length, 1);
});

test("a push is shown and its waitUntil resolves when the windows cannot be listed", async () => {
  const { dispatch, calls } = load({ matchAllError: new Error("clients unavailable") });
  const payload = { id: "i1", title: "Your turn", body: "Game 7", link: "/?game=7" };
  await dispatch("push", pushed(JSON.stringify(payload)));
  assert.equal(calls.show.length, 1);
});

test("notificationclick never leaves the worker's origin", async () => {
  const { dispatch, calls } = load();
  let closed = false;
  await dispatch("notificationclick", {
    notification: {
      data: { link: "//evil.com" },
      close: () => {
        closed = true;
      },
    },
  });
  assert.ok(closed);
  assert.deepEqual(calls.open, [`${ORIGIN}/`]);
});

test("notificationclick focuses an open tab before navigating it", async () => {
  const order: string[] = [];
  const client = {
    url: `${ORIGIN}/other`,
    focus: async () => void order.push("focus"),
    navigate: async (url: string) => void order.push(`navigate ${url}`),
  };
  const { dispatch } = load({ windows: [client] });
  await dispatch("notificationclick", { notification: { data: { link: "/?game=7" }, close: () => {} } });
  assert.deepEqual(order, ["focus", `navigate ${ORIGIN}/?game=7`]);
});

test("notificationclick under a path ignores tabs outside the worker's scope", async () => {
  const focused: string[] = [];
  const tab = (url: string): WindowSpec => ({
    url,
    focus: async () => void focused.push(url),
    navigate: async () => {},
  });
  const { dispatch, calls } = load({
    scope: `${ORIGIN}/app/`,
    windows: [tab(`${ORIGIN}/`), tab(`${ORIGIN}/application`)],
  });
  await dispatch("notificationclick", { notification: { data: { link: "/app/?game=7" }, close: () => {} } });
  assert.deepEqual(focused, []);
  assert.deepEqual(calls.open, [`${ORIGIN}/app/?game=7`]);
});

test("notificationclick under a path focuses a tab inside the worker's scope", async () => {
  const order: string[] = [];
  const client = {
    url: `${ORIGIN}/app/other`,
    focus: async () => void order.push("focus"),
    navigate: async (url: string) => void order.push(`navigate ${url}`),
  };
  const { dispatch } = load({ scope: `${ORIGIN}/app/`, windows: [client] });
  await dispatch("notificationclick", { notification: { data: { link: "/app/?game=7" }, close: () => {} } });
  assert.deepEqual(order, ["focus", `navigate ${ORIGIN}/app/?game=7`]);
});

test("a generic notification opens the worker's scope", async () => {
  const { dispatch, calls } = load({ scope: `${ORIGIN}/app/` });
  await dispatch("push", pushed(null));
  const [, options] = first(calls.show);
  await dispatch("notificationclick", { notification: { data: options["data"], close: () => {} } });
  assert.deepEqual(calls.open, [`${ORIGIN}/app/`]);
});

const body = (call: Call | undefined): unknown => {
  assert.ok(call?.init !== undefined);
  return JSON.parse(call.init.body);
};

test("pushsubscriptionchange registers the new subscription with the server", async () => {
  const { dispatch, calls } = load();
  const json = { endpoint: "E2", keys: { p256dh: "p", auth: "a" } };
  await dispatch("pushsubscriptionchange", { newSubscription: { toJSON: () => json } });
  assert.equal(calls.fetch.length, 1);
  assert.equal(first(calls.fetch).url, "https://gnotif.example/v1/subscription");
  assert.equal(first(calls.fetch).init?.method, "PUT");
  assert.deepEqual(body(first(calls.fetch)), json);
});

test("pushsubscriptionchange names the old endpoint so the opt-ins move", async () => {
  const { dispatch, calls } = load();
  const json = { endpoint: "E2", keys: { p256dh: "p", auth: "a" } };
  await dispatch("pushsubscriptionchange", {
    oldSubscription: { endpoint: "E1" },
    newSubscription: { toJSON: () => json },
  });
  assert.deepEqual(body(first(calls.fetch)), { ...json, oldEndpoint: "E1" });
});

test("pushsubscriptionchange without any subscription subscribes again with the server's key", async () => {
  const { dispatch, calls } = load();
  await dispatch("pushsubscriptionchange", { oldSubscription: null, newSubscription: null });
  assert.deepEqual(plain(calls.subscribe), [{ userVisibleOnly: true, applicationServerKey: "PK" }]);
  assert.deepEqual(body(calls.fetch.find((c) => c.init?.method === "PUT")), RESUBSCRIBED);
});

test("pushsubscriptionchange fails when the server refuses the new subscription", async () => {
  const { dispatch } = load({ putStatus: 400 });
  const json = { endpoint: "E2", keys: { p256dh: "p", auth: "a" } };
  await assert.rejects(dispatch("pushsubscriptionchange", { newSubscription: { toJSON: () => json } }));
});

test("pushsubscriptionchange fails without a server and makes no request", async () => {
  const { dispatch, calls } = load({ search: "" });
  await assert.rejects(dispatch("pushsubscriptionchange", { newSubscription: null, oldSubscription: null }), /server/);
  assert.equal(calls.fetch.length, 0);
});
