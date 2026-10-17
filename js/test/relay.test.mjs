import test from "node:test";
import assert from "node:assert/strict";

import { Gnotif } from "../src/gnotif.js";
import { A, SERVER, connected, last, notification, stubGlobal, stubLive } from "./fakes.mjs";

// stubRelay gives the page a service worker container that captures the
// client's message listener, and returns a function that relays data to it.
// It must run before the client is constructed.
function stubRelay(t) {
  let listener;
  t.after(stubGlobal("navigator", {
    serviceWorker: {
      addEventListener(type, fn) {
        if (type === "message") listener = fn;
      },
    },
  }));
  return (data) => listener({ data });
}

function pushed(link = "/?game=7", id = "i1") {
  const { type, ...fields } = notification(link, id);
  return { type: "gnotif.notification", notification: fields };
}

function relayed(t, options = {}) {
  stubLive(t);
  const relay = stubRelay(t);
  const g = new Gnotif({ server: SERVER, ...options });
  const got = [];
  g.on("notification", (n) => got.push(n));
  return { g, relay, got };
}

test("a relayed push reaches the handler with source push", async (t) => {
  const { g, relay, got } = relayed(t);
  await g.setOptins(A);
  relay(pushed());
  const { type, ...fields } = notification("https://dapp.example/?game=7");
  assert.deepEqual(got, [{ ...fields, source: "push" }]);
});

test("a relayed push outside the set, or before setOptins, reaches no handler", async (t) => {
  const { g, relay, got } = relayed(t);
  relay(pushed());
  await g.setOptins(A);
  const { notification: n } = pushed();
  relay({ type: "gnotif.notification", notification: { ...n, id: "i2", value: "g1alice" } });
  relay({ type: "gnotif.notification", notification: { ...n, id: "i3", trigger: "0000002" } });
  assert.deepEqual(got, []);
});

for (const order of ["live first", "push first"]) {
  test(`each match reaches the handler once, ${order}`, async (t) => {
    stubLive(t);
    const relay = stubRelay(t);
    const g = await connected(t);
    const got = [];
    g.on("notification", (n) => got.push(n));
    const live = () => last().receive(JSON.stringify(notification("/?game=7")));
    const push = () => relay(pushed());
    const [first, second] = order === "live first" ? [live, push] : [push, live];
    first();
    second();
    assert.deepEqual(got.map((n) => n.source), [order === "live first" ? "live" : "push"]);
  });
}

test("a message of another type, or a relay missing a field, reaches no handler", async (t) => {
  const { g, relay, got } = relayed(t);
  await g.setOptins(A);
  const { notification: n } = pushed();
  relay({ type: "other", notification: n });
  relay({ notification: n });
  relay(null);
  relay("gnotif.notification");
  relay({ type: "gnotif.notification" });
  relay({ type: "gnotif.notification", notification: "text" });
  for (const field of Object.keys(n)) {
    const { [field]: dropped, ...partial } = n;
    relay({ type: "gnotif.notification", notification: partial });
  }
  assert.deepEqual(got, []);
});

test("two matches in one transaction both reach the handler", async (t) => {
  const { g, relay, got } = relayed(t);
  await g.setOptins([...A, { trigger: "0000002", value: "g1bob" }]);
  const { notification: n } = pushed();
  relay({ type: "gnotif.notification", notification: n });
  relay({ type: "gnotif.notification", notification: { ...n, id: "i2", trigger: "0000002" } });
  assert.deepEqual(got.map((x) => [x.id, x.trigger, x.tx]), [["i1", "0000001", "tx1"], ["i2", "0000002", "tx1"]]);
});

test("a relayed link to another origin gives the worker's scope", async (t) => {
  const { g, relay, got } = relayed(t, { serviceWorker: "/app/sw.js" });
  await g.setOptins(A);
  relay(pushed("//evil.example/x", "i1"));
  relay(pushed("/?game=7", "i2"));
  assert.equal(got[0]?.link, "https://dapp.example/app/");
  assert.equal(got[1]?.link, "https://dapp.example/?game=7");
});

test("a relay after disconnect still reaches the handler", async (t) => {
  stubLive(t);
  const relay = stubRelay(t);
  const g = await connected(t);
  const got = [];
  g.on("notification", (n) => got.push(n));
  g.disconnect();
  relay(pushed());
  assert.deepEqual(got.map((n) => n.source), ["push"]);
});

test("an id older than the last 256 delivered is delivered again", async (t) => {
  const { g, relay, got } = relayed(t);
  await g.setOptins(A);
  for (let i = 0; i < 257; i++) {
    relay(pushed("/", `i${i}`));
  }
  assert.equal(got.length, 257);
  relay(pushed("/", "i1"));
  assert.equal(got.length, 257);
  relay(pushed("/", "i0"));
  assert.equal(got.length, 258);
});
