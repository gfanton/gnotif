import { test } from "node:test";
import assert from "node:assert/strict";
import { load, save } from "./settings.mjs";

function storage(initial) {
  const items = new Map(initial === undefined ? [] : [["gnotif-demo", initial]]);
  return {
    getItem: (k) => (items.has(k) ? items.get(k) : null),
    setItem: (k, v) => items.set(k, String(v)),
    removeItem: (k) => items.delete(k),
  };
}

const settings = {
  address: "g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5",
  optins: [{ trigger: "0000001", value: "g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5" }],
  live: true,
  push: false,
};

test("saved settings load back", () => {
  const s = storage();
  save(s, settings);
  assert.deepEqual(load(s), settings);
});

test("a record from before the live switch reads as push on, live off", () => {
  const s = storage(JSON.stringify({ address: settings.address, optins: settings.optins }));
  assert.deepEqual(load(s), { address: settings.address, optins: settings.optins, live: false, push: true });
});

test("nothing stored, or garbage, loads as null", () => {
  assert.equal(load(storage()), null);
  assert.equal(load(storage("{")), null);
  assert.equal(load(storage("42")), null);
});
