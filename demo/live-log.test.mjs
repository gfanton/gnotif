import test from "node:test";
import assert from "node:assert/strict";
import { answeredSend, waitText } from "./live-log.mjs";

test("an arrival answers the oldest send waiting for its body", () => {
  const waiting = [{ body: "Hello" }, { body: "Bye" }, { body: "Hello" }];
  assert.equal(answeredSend(waiting, "Hello"), 0);
  assert.equal(answeredSend(waiting, "Bye"), 1);
});

test("an arrival no send waits for answers none", () => {
  assert.equal(answeredSend([{ body: "Hello" }], "Hello, me"), -1);
  assert.equal(answeredSend([], "Hello"), -1);
});

test("the wait reads in whole seconds", () => {
  assert.equal(waitText(6200), "Arrived 6 s after sending");
  assert.equal(waitText(1000), "Arrived 1 s after sending");
  assert.equal(waitText(999), "Arrived within a second of sending");
});
