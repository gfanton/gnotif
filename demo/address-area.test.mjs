import test from "node:test";
import assert from "node:assert/strict";

import { addressArea } from "./address-area.mjs";

const W = "g1zrjyzatudmadgqu39d5wfamhdvf0adwlxkz25d";

test("off without a wallet, the address is typed, and Adena is offered once found", () => {
  assert.deepEqual(addressArea({ on: false, adena: "found", wallet: "" }),
    { input: "edit", wallet: false, disconnect: false, connect: true, getAdena: false, locked: false });
  assert.deepEqual(addressArea({ on: false, adena: "absent", wallet: "" }),
    { input: "edit", wallet: false, disconnect: false, connect: false, getAdena: true, locked: false });
});

test("while Adena is still being looked for, neither Connect nor Get Adena shows", () => {
  const area = addressArea({ on: false, adena: "pending", wallet: "" });
  assert.equal(area.connect, false);
  assert.equal(area.getAdena, false);
});

test("a connected wallet replaces the input until it is disconnected", () => {
  assert.deepEqual(addressArea({ on: false, adena: "found", wallet: W }),
    { input: "hidden", wallet: true, disconnect: true, connect: false, getAdena: false, locked: false });
});

test("notifications on lock the address, typed or from the wallet", () => {
  assert.deepEqual(addressArea({ on: true, adena: "absent", wallet: "" }),
    { input: "readonly", wallet: false, disconnect: false, connect: false, getAdena: false, locked: true });
  assert.deepEqual(addressArea({ on: true, adena: "found", wallet: W }),
    { input: "hidden", wallet: true, disconnect: false, connect: false, getAdena: false, locked: true });
});
