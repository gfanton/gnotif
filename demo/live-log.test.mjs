import test from "node:test";
import assert from "node:assert/strict";
import { answeredSend, blockSummary, clockText, details, waitText } from "./live-log.mjs";

test("an arrival answers the oldest send waiting for its body", () => {
  const waiting = [{ body: "Hello" }, { body: "Bye" }, { body: "Hello" }];
  assert.equal(answeredSend(waiting, "Hello"), 0);
  assert.equal(answeredSend(waiting, "Bye"), 1);
});

test("an arrival no send waits for answers none", () => {
  assert.equal(answeredSend([{ body: "Hello" }], "Hello, me"), -1);
  assert.equal(answeredSend([], "Hello"), -1);
});

test("the waiting clock reads minutes and whole seconds", () => {
  assert.equal(clockText(0), "0:00");
  assert.equal(clockText(4200), "0:04");
  assert.equal(clockText(59_999), "0:59");
  assert.equal(clockText(60_000), "1:00");
  assert.equal(clockText(75_000), "1:15");
});

test("the summary names the block, with the wait when a send waited for it", () => {
  assert.equal(blockSummary(181221, 6200), "Block 181221 · Arrived 6 s after sending");
  assert.equal(blockSummary(181221, undefined), "Block 181221");
});

const echoed = {
  trigger: "0000001", value: "g1bob", title: "Echo", body: "Hello", link: "https://dapp.example/",
  height: 181221, tx: "aGFzaDE=", time: "2026-10-16T21:46:08.5Z",
  attrs: [{ key: "to", value: "g1bob" }, { key: "msg", value: "Hello" }],
};
const iso = (d) => d.toISOString();

test("the details give the block time, the transaction, the trigger, the event's attributes and the link", () => {
  assert.deepEqual(details(echoed, iso), [
    ["Block time", "2026-10-16T21:46:08.500Z"],
    ["Transaction", "aGFzaDE="],
    ["Trigger", "0000001 for g1bob"],
    ["to", "g1bob"],
    ["msg", "Hello"],
    ["Link", "https://dapp.example/"],
  ]);
});

test("the details leave out a block time that does not parse and a trigger's empty value", () => {
  assert.deepEqual(details({ ...echoed, value: "", time: "yesterday", attrs: [] }, iso), [
    ["Transaction", "aGFzaDE="],
    ["Trigger", "0000001"],
    ["Link", "https://dapp.example/"],
  ]);
});

test("the wait reads in whole seconds", () => {
  assert.equal(waitText(6200), "Arrived 6 s after sending");
  assert.equal(waitText(1000), "Arrived 1 s after sending");
  assert.equal(waitText(999), "Arrived within a second of sending");
});
