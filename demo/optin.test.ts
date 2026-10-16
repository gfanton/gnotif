import { test } from "node:test";
import assert from "node:assert/strict";
import { canEnable, listenerMismatch, optinsAtLoad, optinsFor } from "./optin.ts";

const trigger = {
  id: "7",
  target: "gno.land/r/dev/echo/v1",
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
  assert.throws(() => optinsFor(null, address), {
    message: "The echo trigger has not loaded. Reload the page, then try again.",
  });
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

test("optinsAtLoad sets the opt-ins as the page loads once push is on and the trigger has loaded", () => {
  assert.equal(optinsAtLoad(true, false, trigger), true);
  assert.equal(optinsAtLoad(false, false, trigger), false, "push off");
  assert.equal(optinsAtLoad(true, false, null), false, "no trigger");
});

test("optinsAtLoad leaves the opt-ins to live updates that resume at load", () => {
  assert.equal(optinsAtLoad(true, true, trigger), false);
});

test("listenerMismatch names the opted-in address when the sender is another", () => {
  assert.equal(listenerMismatch(address, true, { address: other }), other);
});

test("listenerMismatch is undefined when this browser listens for the sender or for nobody", () => {
  assert.equal(listenerMismatch(address, true, { address }), undefined, "same address");
  assert.equal(listenerMismatch(address, false, { address: other }), undefined, "notifications off");
  assert.equal(listenerMismatch(address, true, null), undefined, "nothing stored");
});
