import test, { type TestContext } from "node:test";
import assert from "node:assert/strict";

import { Gnotif, GnotifError, base64UrlToBytes, sameKey } from "../src/gnotif.ts";

const SERVER = "https://gnotif.example";

function stubGlobal(name: string, value: unknown): () => void {
  const previous = Object.getOwnPropertyDescriptor(globalThis, name);
  Object.defineProperty(globalThis, name, { value, configurable: true, writable: true });
  return () => {
    if (previous !== undefined) Object.defineProperty(globalThis, name, previous);
    else Reflect.deleteProperty(globalThis, name);
  };
}

type Answer = { ok: boolean; status: number; json: () => Promise<unknown> };

test("base64UrlToBytes decodes unpadded base64url", () => {
  assert.deepEqual([...base64UrlToBytes("AQID")], [1, 2, 3]);
  assert.deepEqual([...base64UrlToBytes("-_8")], [251, 255]);
});

test("sameKey compares an ArrayBuffer with bytes", () => {
  const key = new Uint8Array([1, 2, 3]);
  assert.equal(sameKey(new Uint8Array([1, 2, 3]).buffer, key), true);
  assert.equal(sameKey(new Uint8Array([1, 2, 4]).buffer, key), false);
  assert.equal(sameKey(null, key), false);
});

test("enable throws unsupported without a service worker API", async (t) => {
  t.after(stubGlobal("navigator", {}));
  const g = new Gnotif({ server: SERVER, serviceWorker: "/sw.js" });
  await assert.rejects(g.enable(), (err) => err instanceof GnotifError && err.code === "unsupported");
});

test("enable throws denied when permission is refused", async (t) => {
  t.after(stubGlobal("navigator", { serviceWorker: {} }));
  t.after(stubGlobal("PushManager", function PushManager() {}));
  t.after(stubGlobal("Notification", { requestPermission: async () => "denied" }));
  const g = new Gnotif({ server: SERVER, serviceWorker: "/sw.js" });
  await assert.rejects(g.enable(), (err) => err instanceof GnotifError && err.code === "denied");
});

test("setOptins sends the endpoint and opt-ins, and surfaces server errors", async (t) => {
  const subscription = { endpoint: "E" };
  stubBrowser(t, { permission: "granted", subscription });
  const requests: { url: string; init: RequestInit }[] = [];
  let answer: Answer = { ok: true, status: 204, json: async () => null };
  t.after(
    stubGlobal("fetch", async (url: string, init: RequestInit) => {
      requests.push({ url, init });
      return answer;
    }),
  );

  const g = new Gnotif({ server: SERVER, serviceWorker: "/sw.js" });
  const optins = [{ trigger: "0000001", value: "g1bob" }];
  await g.setOptins(optins);
  assert.equal(requests.length, 1);
  assert.equal(requests[0]?.url, `${SERVER}/v1/subscription/optins`);
  assert.equal(requests[0]?.init.method, "PUT");
  const body = requests[0]?.init.body;
  assert.ok(typeof body === "string");
  assert.deepEqual(JSON.parse(body), { endpoint: "E", optins });

  answer = { ok: false, status: 400, json: async () => ({ error: "unknown trigger" }) };
  await assert.rejects(
    g.setOptins(optins),
    (err) => err instanceof GnotifError && err.code === "server" && err.message === "unknown trigger",
  );
});

// stubBrowser fakes a page that is controlled by a registration holding
// subscription. Without one, getRegistration resolves undefined and ready
// never settles, as in a browser.
function stubBrowser(
  t: TestContext,
  { permission, subscription, registered = true }: { permission: string; subscription: unknown; registered?: boolean },
): void {
  const reg = { pushManager: { getSubscription: async () => subscription } };
  t.after(
    stubGlobal("navigator", {
      serviceWorker: {
        getRegistration: async () => (registered ? reg : undefined),
        ready: registered ? Promise.resolve(reg) : new Promise(() => {}),
      },
    }),
  );
  t.after(stubGlobal("PushManager", function PushManager() {}));
  t.after(stubGlobal("Notification", { permission }));
}

test("enabled is true with a subscription and granted permission", async (t) => {
  stubBrowser(t, { permission: "granted", subscription: { endpoint: "E" } });
  assert.equal(await new Gnotif({ server: SERVER }).enabled(), true);
});

test("enabled is false without a subscription", async (t) => {
  stubBrowser(t, { permission: "granted", subscription: null });
  assert.equal(await new Gnotif({ server: SERVER }).enabled(), false);
});

test("enabled is false when permission was revoked while a subscription exists", async (t) => {
  for (const permission of ["denied", "default"]) {
    stubBrowser(t, { permission, subscription: { endpoint: "E" } });
    assert.equal(await new Gnotif({ server: SERVER }).enabled(), false, permission);
  }
});

test("disable unsubscribes the browser even when the server answers 404", async (t) => {
  let unsubscribed = false;
  const subscription = {
    endpoint: "E",
    unsubscribe: async () => {
      unsubscribed = true;
      return true;
    },
  };
  stubBrowser(t, { permission: "granted", subscription });
  const requests: { url: string; init: RequestInit }[] = [];
  t.after(
    stubGlobal("fetch", async (url: string, init: RequestInit) => {
      requests.push({ url, init });
      return { ok: false, status: 404, json: async () => ({ error: "unknown subscription" }) };
    }),
  );
  await new Gnotif({ server: SERVER }).disable();
  assert.ok(unsubscribed);
  assert.equal(requests.length, 1);
  assert.equal(requests[0]?.init.method, "DELETE");
});

test("enabled is false when no service worker registration covers the page", async (t) => {
  stubBrowser(t, { permission: "granted", subscription: null, registered: false });
  assert.equal(await new Gnotif({ server: SERVER }).enabled(), false);
});

test("disable rejects as inactive when no registration covers the page", async (t) => {
  stubBrowser(t, { permission: "granted", subscription: null, registered: false });
  const g = new Gnotif({ server: SERVER });
  await assert.rejects(g.disable(), (err) => err instanceof GnotifError && err.code === "inactive");
});

test("setOptins without push keeps the set and sends nothing", async (t) => {
  stubBrowser(t, { permission: "granted", subscription: null, registered: false });
  const requests: { url: string; init: RequestInit }[] = [];
  t.after(
    stubGlobal("fetch", async (url: string, init: RequestInit) => {
      requests.push({ url, init });
      return { ok: true, status: 204, json: async () => null };
    }),
  );
  await new Gnotif({ server: SERVER }).setOptins([{ trigger: "0000001", value: "g1bob" }]);
  assert.equal(requests.length, 0);
});

// stubEnable fakes everything enable() touches: a granted prompt, a service
// worker, a push manager without a subscription, and a server answering
// /v1/vapid with vapid. It returns the requests the server received.
function stubEnable(
  t: TestContext,
  vapid: unknown = { publicKey: "AQID" },
): { url: string; method: string; body: unknown }[] {
  const sub = { endpoint: "E", options: {}, toJSON: () => ({ endpoint: "E", keys: {} }) };
  let current: typeof sub | null = null;
  const reg = {
    pushManager: {
      getSubscription: async () => current,
      subscribe: async () => {
        current = sub;
        return sub;
      },
    },
  };
  t.after(
    stubGlobal("navigator", {
      serviceWorker: { register: async () => reg, ready: Promise.resolve(reg), getRegistration: async () => reg },
    }),
  );
  t.after(stubGlobal("PushManager", function PushManager() {}));
  t.after(stubGlobal("Notification", { permission: "granted", requestPermission: async () => "granted" }));
  const requests: { url: string; method: string; body: unknown }[] = [];
  t.after(
    stubGlobal("fetch", async (url: string, init: RequestInit = {}) => {
      const body: unknown = typeof init.body === "string" ? JSON.parse(init.body) : undefined;
      requests.push({ url, method: init.method ?? "GET", body });
      if (url.endsWith("/v1/vapid")) {
        return { ok: true, status: 200, json: async () => vapid };
      }
      return { ok: true, status: 204, json: async () => null };
    }),
  );
  return requests;
}

test("enable registers the kept opt-ins", async (t) => {
  const requests = stubEnable(t);
  const g = new Gnotif({ server: SERVER });
  const optins = [{ trigger: "0000001", value: "g1bob" }];
  await g.setOptins(optins);
  await g.enable();
  assert.deepEqual(
    requests.map((r) => `${r.method} ${r.url}`),
    [`GET ${SERVER}/v1/vapid`, `PUT ${SERVER}/v1/subscription`, `PUT ${SERVER}/v1/subscription/optins`],
  );
  assert.deepEqual(requests[2]?.body, { endpoint: "E", optins });
});

test("enable without a kept set sends no opt-ins", async (t) => {
  const requests = stubEnable(t);
  await new Gnotif({ server: SERVER }).enable();
  assert.deepEqual(
    requests.map((r) => `${r.method} ${r.url}`),
    [`GET ${SERVER}/v1/vapid`, `PUT ${SERVER}/v1/subscription`],
  );
});

test("enable rejects a malformed VAPID answer", async (t) => {
  stubEnable(t, { key: "PK" });
  const g = new Gnotif({ server: SERVER, serviceWorker: "/sw.js" });
  await assert.rejects(g.enable(), (err) => err instanceof GnotifError && err.code === "server");
});

for (const publicKey of ["", "!!", "A"]) {
  test(`enable rejects the VAPID key ${JSON.stringify(publicKey)}, which is not base64url`, async (t) => {
    stubEnable(t, { publicKey });
    const g = new Gnotif({ server: SERVER, serviceWorker: "/sw.js" });
    await assert.rejects(g.enable(), (err) => err instanceof GnotifError && err.code === "server");
  });
}

const BOB = { trigger: "0000001", value: "g1bob" };
const BOB_PAGE_ONLY = { trigger: "0000002", value: "g1bob", push: false };

test("setOptins sends only the push items to the push registration", async (t) => {
  const requests = stubEnable(t);
  const g = new Gnotif({ server: SERVER });
  await g.enable();
  await g.setOptins([BOB, BOB_PAGE_ONLY]);
  assert.deepEqual(requests.at(-1)?.body, { endpoint: "E", optins: [BOB] });
});

test("enable registers only the push items of the kept set", async (t) => {
  const requests = stubEnable(t);
  const g = new Gnotif({ server: SERVER });
  await g.setOptins([BOB, BOB_PAGE_ONLY]);
  await g.enable();
  assert.deepEqual(requests.at(-1)?.body, { endpoint: "E", optins: [BOB] });
});

test("a set of page-only items empties the push registration", async (t) => {
  const requests = stubEnable(t);
  const g = new Gnotif({ server: SERVER });
  await g.enable();
  await g.setOptins([BOB_PAGE_ONLY]);
  assert.deepEqual(requests.at(-1)?.body, { endpoint: "E", optins: [] });
});

test("an item listed twice is pushed when one copy allows push", async (t) => {
  const requests = stubEnable(t);
  const g = new Gnotif({ server: SERVER });
  await g.enable();
  await g.setOptins([
    { ...BOB, push: false },
    { ...BOB, push: true },
  ]);
  assert.deepEqual(requests.at(-1)?.body, { endpoint: "E", optins: [BOB] });
});

test("enabled is false without the Push API", async (t) => {
  stubBrowser(t, { permission: "granted", subscription: { endpoint: "E" } });
  t.after(stubGlobal("PushManager", undefined));
  Reflect.deleteProperty(globalThis, "PushManager");
  assert.equal(await new Gnotif({ server: SERVER }).enabled(), false);
});

test("triggers returns the server's list", async (t) => {
  const list = [
    {
      id: "t1",
      target: "gno.land/r/demo/game",
      event: "TurnPlayed",
      filter: "",
      param: "",
      title: "Your turn",
      body: "",
      link: "/",
      declarer: "g1game",
      verified: true,
    },
  ];
  t.after(stubGlobal("fetch", async () => ({ ok: true, status: 200, json: async () => list })));
  assert.deepEqual(await new Gnotif({ server: SERVER }).triggers("gno.land/r/demo/game"), list);
});

test("triggers encodes the target in the query string", async (t) => {
  const urls: string[] = [];
  t.after(
    stubGlobal("fetch", async (url: string) => {
      urls.push(url);
      return { ok: true, status: 200, json: async () => [] };
    }),
  );
  await new Gnotif({ server: SERVER }).triggers("gno.land/r/a b");
  assert.deepEqual(urls, [`${SERVER}/v1/triggers?target=gno.land%2Fr%2Fa%20b`]);
});

test("triggers rejects a target that is not a non-empty string before any request", async (t) => {
  let calls = 0;
  t.after(
    stubGlobal("fetch", async () => {
      calls++;
      return { ok: true, status: 200, json: async () => [] };
    }),
  );
  const gnotif = new Gnotif({ server: SERVER });
  for (const target of [undefined, "", 7]) {
    // Reflect.apply calls triggers as a JavaScript caller can, with any target.
    const call: unknown = Reflect.apply(gnotif.triggers.bind(gnotif), undefined, [target]);
    assert.ok(call instanceof Promise);
    await assert.rejects(call, TypeError);
  }
  assert.equal(calls, 0);
});

const TRIGGER = {
  id: "0000001",
  target: "gno.land/r/dev/echo/v0",
  event: "Echo",
  filter: "",
  param: "to",
  title: "Echo",
  body: "{msg}",
  link: "/",
  declarer: "g1x",
  verified: true,
};
const answering = (body: unknown) => async () => ({ ok: true, status: 200, json: async () => body });

test("triggers rejects a malformed answer", async (t) => {
  t.after(stubGlobal("fetch", answering([{ id: 7 }])));
  await assert.rejects(
    new Gnotif({ server: SERVER }).triggers("gno.land/r/dev/echo/v0"),
    (err) => err instanceof GnotifError && err.code === "server",
  );
});

test("triggers rejects an answer that is not JSON", async (t) => {
  t.after(
    stubGlobal("fetch", async () => ({
      ok: true,
      status: 200,
      json: async () => {
        throw new SyntaxError("Unexpected token '<'");
      },
    })),
  );
  await assert.rejects(
    new Gnotif({ server: SERVER }).triggers("gno.land/r/dev/echo/v0"),
    (err) => err instanceof GnotifError && err.code === "server",
  );
});

test("a server error whose body has no string error gives the status message", async (t) => {
  t.after(stubGlobal("fetch", async () => ({ ok: false, status: 400, json: async () => ({ error: 5 }) })));
  await assert.rejects(
    new Gnotif({ server: SERVER }).triggers("gno.land/r/dev/echo/v0"),
    (err) =>
      err instanceof GnotifError &&
      err.code === "server" &&
      err.status === 400 &&
      err.message === "gnotif server answered 400",
  );
});

test("triggers keeps the known fields of an answer with an extra field", async (t) => {
  t.after(stubGlobal("fetch", answering([{ ...TRIGGER, icon: "/i.png" }])));
  assert.deepEqual(await new Gnotif({ server: SERVER }).triggers("gno.land/r/dev/echo/v0"), [TRIGGER]);
});

test("no options use the onyx server", () => {
  const g = new Gnotif();
  assert.equal(g.network, "onyx");
  assert.equal(g.server, "https://gnotif.xyz/onyx");
});

test("network onyx resolves to the hosted onyx server", async (t) => {
  const requests: string[] = [];
  t.after(
    stubGlobal("fetch", async (url: string) => {
      requests.push(url);
      return { ok: true, status: 200, json: async () => [] };
    }),
  );
  await new Gnotif({ network: "onyx" }).triggers("gno.land/r/dev/echo/v0");
  assert.equal(requests[0], "https://gnotif.xyz/onyx/v1/triggers?target=gno.land%2Fr%2Fdev%2Fecho%2Fv0");
});

test("an unknown network throws and names the known ones", () => {
  for (const network of ["testnet", "constructor", "toString", ""]) {
    assert.throws(() => new Gnotif({ network }), {
      name: "TypeError",
      message: `unknown network "${network}": use "onyx", or pass server`,
    });
  }
});

test("server wins over network and keeps its path", () => {
  const g = new Gnotif({ network: "onyx", server: "https://x.example/onyx/" });
  assert.equal(g.server, "https://x.example/onyx");
  assert.equal(g.network, "onyx");
  assert.equal(new Gnotif({ server: "http://localhost:8080" }).network, undefined);
});

test("enable registers the service worker with the network's server", async (t) => {
  let registered: unknown;
  t.after(
    stubGlobal("navigator", {
      serviceWorker: {
        register: async (url: string) => {
          registered = url;
          throw new Error("stop");
        },
      },
    }),
  );
  t.after(stubGlobal("PushManager", function PushManager() {}));
  t.after(stubGlobal("Notification", { requestPermission: async () => "granted" }));
  await assert.rejects(new Gnotif({ network: "onyx" }).enable(), /stop/);
  assert.equal(registered, "/sw.js?server=https%3A%2F%2Fgnotif.xyz%2Fonyx");
});

test("enable registers the service worker with the icon resolved against the page", async (t) => {
  let registered: unknown;
  t.after(
    stubGlobal("navigator", {
      serviceWorker: {
        register: async (url: string) => {
          registered = url;
          throw new Error("stop");
        },
      },
    }),
  );
  t.after(stubGlobal("PushManager", function PushManager() {}));
  t.after(stubGlobal("Notification", { requestPermission: async () => "granted" }));
  t.after(stubGlobal("document", { baseURI: "https://dapp.example/app/" }));
  await assert.rejects(new Gnotif({ network: "onyx", icon: "icon.png" }).enable(), /stop/);
  assert.equal(
    registered,
    "/sw.js?server=https%3A%2F%2Fgnotif.xyz%2Fonyx&icon=https%3A%2F%2Fdapp.example%2Fapp%2Ficon.png",
  );
});
