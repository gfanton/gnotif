import test from "node:test";
import assert from "node:assert/strict";

import { addressArea, lockedRefusal } from "./address-area.ts";

const W = "g1zrjyzatudmadgqu39d5wfamhdvf0adwlxkz25d";
const OTHER = "g13rqvks880v0hmlfepcj0kkudq0js38d3pc5k6a";

test("off without a wallet, the address is typed, and Adena is offered once found", () => {
  assert.deepEqual(addressArea({ on: false, adena: "found", wallet: "" }), {
    input: "edit",
    wallet: false,
    disconnect: false,
    connect: true,
    getAdena: false,
    locked: false,
  });
  assert.deepEqual(addressArea({ on: false, adena: "absent", wallet: "" }), {
    input: "edit",
    wallet: false,
    disconnect: false,
    connect: false,
    getAdena: true,
    locked: false,
  });
});

test("while Adena is still being looked for, neither Connect nor Get Adena shows", () => {
  const area = addressArea({ on: false, adena: "pending", wallet: "" });
  assert.equal(area.connect, false);
  assert.equal(area.getAdena, false);
});

test("a connected wallet replaces the input until it is disconnected", () => {
  assert.deepEqual(addressArea({ on: false, adena: "found", wallet: W }), {
    input: "hidden",
    wallet: true,
    disconnect: true,
    connect: false,
    getAdena: false,
    locked: false,
  });
});

test("notifications on lock the address, typed or from the wallet", () => {
  assert.deepEqual(addressArea({ on: true, adena: "absent", wallet: "" }), {
    input: "readonly",
    wallet: false,
    disconnect: false,
    connect: false,
    getAdena: false,
    locked: true,
  });
  assert.deepEqual(addressArea({ on: true, adena: "found", wallet: W }), {
    input: "hidden",
    wallet: true,
    disconnect: false,
    connect: false,
    getAdena: false,
    locked: true,
  });
});

test("with notifications on, Adena can connect again, so a returning visitor can send with it", () => {
  assert.deepEqual(addressArea({ on: true, adena: "found", wallet: "" }), {
    input: "readonly",
    wallet: false,
    disconnect: false,
    connect: true,
    getAdena: false,
    locked: true,
  });
});

test("a locked address takes Adena's account only when it is that address", () => {
  assert.equal(lockedRefusal(true, W, W), "");
  assert.equal(
    lockedRefusal(true, W, OTHER),
    "Adena's active account is g13rqv…5k6a, but notifications are on for g1zrjy…z25d. Switch Adena to that account, or turn notifications off first.",
  );
  assert.equal(lockedRefusal(false, W, OTHER), "", "an address that is not locked takes any account");
});
