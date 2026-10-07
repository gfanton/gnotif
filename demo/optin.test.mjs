import { test } from "node:test";
import assert from "node:assert/strict";
import { canEnable, listenerMismatch, optinsFor } from "./optin.mjs";

const trigger = {
  id: "7",
  target: "gno.land/r/dev/echo/v0",
  event: "Echo",
  filter: "",
  param: "to",
  title: "Echo",
  body: "{msg}",
  link: "/",
  declarer: "g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5",
  verified: true,
};
const address = "g1zrjyzatudmadgqu39d5wfamhdvf0adwlxkz25d";
const other = "g13rqvks880v0hmlfepcj0kkudq0js38d3pc5k6a";

test("optinsFor opts the address into the trigger", () => {
  assert.deepEqual(optinsFor(trigger, address), [{ trigger: "7", value: address }]);
});

test("optinsFor refuses before the trigger has loaded", () => {
  assert.throws(() => optinsFor(null, address), { message: "The echo trigger has not loaded. Reload the page, then try again." });
});

test("optinsFor refuses an empty or malformed address", () => {
  assert.throws(() => optinsFor(trigger, ""), { message: "Enter your address, or connect Adena." });
  assert.throws(() => optinsFor(trigger, "g1nope"), { message: /^"g1nope" isn't a valid gno address/ });
});

test("canEnable needs an idle page and a loaded trigger", () => {
  assert.equal(canEnable(false, trigger), true);
  assert.equal(canEnable(true, trigger), false, "busy");
  assert.equal(canEnable(false, null), false, "no trigger");
});

test("listenerMismatch names the opted-in address when the sender is another", () => {
  assert.equal(listenerMismatch(address, true, { address: other, optins: [] }), other);
});

test("listenerMismatch is undefined when this browser listens for the sender or for nobody", () => {
  assert.equal(listenerMismatch(address, true, { address, optins: [] }), undefined, "same address");
  assert.equal(listenerMismatch(address, false, { address: other, optins: [] }), undefined, "notifications off");
  assert.equal(listenerMismatch(address, true, null), undefined, "nothing stored");
});
