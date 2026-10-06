import test from "node:test";
import assert from "node:assert/strict";

import { Gnotif, GnotifError, base64UrlToBytes, sameKey } from "../src/gnotif.js";

const SERVER = "https://gnotif.example";

function stubGlobal(name, value) {
  const previous = Object.getOwnPropertyDescriptor(globalThis, name);
  Object.defineProperty(globalThis, name, { value, configurable: true, writable: true });
  return () => {
    if (previous) Object.defineProperty(globalThis, name, previous);
    else delete globalThis[name];
  };
}

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
  const requests = [];
  let answer = { ok: true, status: 204, json: async () => null };
  t.after(stubGlobal("fetch", async (url, init) => {
    requests.push({ url, init });
    return answer;
  }));

  const g = new Gnotif({ server: SERVER, serviceWorker: "/sw.js" });
  const optins = [{ trigger: "0000001", value: "g1bob" }];
  await g.setOptins(optins);
  assert.equal(requests.length, 1);
  assert.equal(requests[0].url, `${SERVER}/v1/subscription/optins`);
  assert.equal(requests[0].init.method, "PUT");
  assert.deepEqual(JSON.parse(requests[0].init.body), { endpoint: "E", optins });

  answer = { ok: false, status: 400, json: async () => ({ error: "unknown trigger" }) };
  await assert.rejects(g.setOptins(optins), (err) =>
    err instanceof GnotifError && err.code === "server" && err.message === "unknown trigger");
});

// stubBrowser fakes a page that is controlled by a registration holding
// subscription. Without one, getRegistration resolves undefined and ready
// never settles, as in a browser.
function stubBrowser(t, { permission, subscription, registered = true }) {
  const reg = { pushManager: { getSubscription: async () => subscription } };
  t.after(stubGlobal("navigator", {
    serviceWorker: {
      getRegistration: async () => (registered ? reg : undefined),
      ready: registered ? Promise.resolve(reg) : new Promise(() => {}),
    },
  }));
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
  const subscription = { endpoint: "E", unsubscribe: async () => { unsubscribed = true; return true; } };
  stubBrowser(t, { permission: "granted", subscription });
  const requests = [];
  t.after(stubGlobal("fetch", async (url, init) => {
    requests.push({ url, init });
    return { ok: false, status: 404, json: async () => ({ error: "unknown subscription" }) };
  }));
  await new Gnotif({ server: SERVER }).disable();
  assert.ok(unsubscribed);
  assert.equal(requests.length, 1);
  assert.equal(requests[0].init.method, "DELETE");
});

test("enabled is false when no service worker registration covers the page", async (t) => {
  stubBrowser(t, { permission: "granted", subscription: null, registered: false });
  assert.equal(await new Gnotif({ server: SERVER }).enabled(), false);
});

test("disable and setOptins reject as inactive when no registration covers the page", async (t) => {
  stubBrowser(t, { permission: "granted", subscription: null, registered: false });
  const g = new Gnotif({ server: SERVER });
  const inactive = (err) => err instanceof GnotifError && err.code === "inactive";
  await assert.rejects(g.disable(), inactive);
  await assert.rejects(g.setOptins([]), inactive);
});

test("enabled is false without the Push API", async (t) => {
  stubBrowser(t, { permission: "granted", subscription: { endpoint: "E" } });
  t.after(stubGlobal("PushManager", undefined));
  delete globalThis.PushManager;
  assert.equal(await new Gnotif({ server: SERVER }).enabled(), false);
});

test("triggers returns the server's list", async (t) => {
  const list = [{ id: "t1", target: "gno.land/r/demo/game", event: "TurnPlayed", filter: "", param: "",
    title: "Your turn", body: "", link: "/", declarer: "g1game", verified: true }];
  t.after(stubGlobal("fetch", async () => ({ ok: true, status: 200, json: async () => list })));
  assert.deepEqual(await new Gnotif({ server: SERVER }).triggers("gno.land/r/demo/game"), list);
});

test("triggers encodes the target in the query string", async (t) => {
  const urls = [];
  t.after(stubGlobal("fetch", async (url) => {
    urls.push(url);
    return { ok: true, status: 200, json: async () => [] };
  }));
  await new Gnotif({ server: SERVER }).triggers("gno.land/r/a b");
  assert.deepEqual(urls, [`${SERVER}/v1/triggers?target=gno.land%2Fr%2Fa%20b`]);
});

test("triggers rejects a target that is not a non-empty string before any request", async (t) => {
  let calls = 0;
  t.after(stubGlobal("fetch", async () => {
    calls++;
    return { ok: true, status: 200, json: async () => [] };
  }));
  const gnotif = new Gnotif({ server: SERVER });
  for (const target of [undefined, "", 7]) {
    await assert.rejects(gnotif.triggers(/** @type {any} */ (target)), TypeError);
  }
  assert.equal(calls, 0);
});
