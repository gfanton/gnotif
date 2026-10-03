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
  t.after(stubGlobal("navigator", {
    serviceWorker: { ready: Promise.resolve({ pushManager: { getSubscription: async () => subscription } }) },
  }));
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
