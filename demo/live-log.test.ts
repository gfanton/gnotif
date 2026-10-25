import test from "node:test";
import assert from "node:assert/strict";
import type { GnotifNotification } from "gnotif";
import {
  answeredSend,
  arrivedText,
  bannerCheck,
  blockSummary,
  bodyFor,
  deliveredBody,
  details,
  lateAfter,
  lateNote,
  lateRowNote,
  logShown,
  sentText,
  waitOf,
  waitText,
  waitsFor,
} from "./live-log.ts";

test("an arrival answers the oldest send waiting for its body", () => {
  const waiting = [{ body: "Hello" }, { body: "Bye" }, { body: "Hello" }];
  assert.equal(answeredSend(waiting, "Hello"), 0);
  assert.equal(answeredSend(waiting, "Bye"), 1);
});

test("an arrival answers a send still on time before a late one with its body", () => {
  assert.equal(answeredSend([{ body: "Hello", late: true }, { body: "Hello" }], "Hello"), 1);
  assert.equal(answeredSend([{ body: "Hello", late: true }, { body: "Bye" }], "Hello"), 0);
});

test("an arrival no send waits for answers none", () => {
  assert.equal(answeredSend([{ body: "Hello" }], "Hello, me"), -1);
  assert.equal(answeredSend([], "Hello"), -1);
});

test("a body over 255 UTF-8 bytes is cut on a character boundary, as gnotifd delivers it", () => {
  const ascii = "a".repeat(200);
  assert.equal(deliveredBody(ascii), ascii);
  assert.equal(deliveredBody("a" + "é".repeat(127)), "a" + "é".repeat(127)); // 255 bytes
  assert.equal(deliveredBody("é".repeat(128)), "é".repeat(127)); // 256 bytes
  assert.equal(deliveredBody("界".repeat(86)), "界".repeat(85)); // 258 bytes
  assert.equal(deliveredBody("🙂".repeat(64)), "🙂".repeat(63)); // 256 bytes
});

test("the summary names the block, with the wait when a send waited for it", () => {
  assert.equal(blockSummary(181221, 6200), "Block 181221 · Arrived 6\u00a0s after sending");
  assert.equal(blockSummary(181221, undefined), "Block 181221");
});

const echoed: GnotifNotification = {
  id: "0123456789abcdef0123456789abcdef",
  source: "live",
  trigger: "0000001",
  value: "g1bob",
  title: "Echo",
  body: "Hello",
  link: "https://dapp.example/",
  height: 181221,
  tx: "aGFzaDE=",
  time: "2026-10-16T21:46:08.5Z",
  attrs: [
    { key: "to", value: "g1bob" },
    { key: "msg", value: "Hello" },
  ],
};
const iso = (d: Date): string => d.toISOString();

test("the details give the block time, the transaction, the trigger, the event's attributes and the link", () => {
  assert.deepEqual(details(echoed, iso), [
    ["Block time", "2026-10-16T21:46:08.500Z"],
    ["Transaction", "aGFzaDE="],
    ["Delivered by", "live connection"],
    ["Trigger", "0000001 for g1bob"],
    ["to", "g1bob"],
    ["msg", "Hello"],
    ["Link", "https://dapp.example/"],
  ]);
});

test("the details leave out a block time that does not parse and a trigger's empty value", () => {
  assert.deepEqual(details({ ...echoed, value: "", time: "yesterday", attrs: [] }, iso), [
    ["Transaction", "aGFzaDE="],
    ["Delivered by", "live connection"],
    ["Trigger", "0000001"],
    ["Link", "https://dapp.example/"],
  ]);
});

test("the details say a push delivered the notification", () => {
  assert.deepEqual(
    details({ ...echoed, source: "push" }, iso).filter(([label]) => label === "Delivered by"),
    [["Delivered by", "push"]],
  );
});

test("the log shows while live updates are on, and with them off once a push lists a row", () => {
  assert.equal(logShown(true, 0), true);
  assert.equal(logShown(true, 2), true);
  assert.equal(logShown(false, 1), true);
  assert.equal(logShown(false, 0), false);
});

test("the wait reads in whole seconds, its number never apart from its unit", () => {
  assert.equal(waitText(6200), "Arrived 6\u00a0s after sending");
  assert.equal(waitText(1000), "Arrived 1\u00a0s after sending");
  assert.equal(waitText(999), "Arrived within a second of sending");
});

test("a late send names the check for the way it was sent", () => {
  assert.equal(
    lateNote("adena"),
    "It has not arrived after 60\u00a0s. Check that Adena sends from the address in step 1.",
  );
  assert.equal(
    lateNote("gnokey"),
    "It has not arrived after 3\u00a0min. If you ran the command, check that it used the key of the address in step 1, and that the address has test GNOT.",
  );
});

test("a late row only says how long it waited, since step 2's line holds the checks", () => {
  assert.equal(lateRowNote("adena"), "Not arrived after 60 s");
  assert.equal(lateRowNote("gnokey"), "Not arrived after 3 min");
});

test("a gnokey send waits longer before it reads as late, since the visitor still pastes and signs it", () => {
  assert.equal(lateAfter("adena"), 60_000);
  assert.equal(lateAfter("gnokey"), 180_000);
});

test("only an Adena send has a wait to report, since a gnokey send is timed from the copy", () => {
  assert.equal(waitOf({ via: "adena", at: 1_000 }, 5_200), 4_200);
  assert.equal(waitOf({ via: "gnokey", at: 1_000 }, 5_200), undefined);
});

test("an answered send names the message that arrived, and its wait when known", () => {
  assert.equal(arrivedText(4200, "Hello"), 'Arrived in 4\u00a0s: "Hello"');
  assert.equal(arrivedText(400, "Hello"), 'Arrived within a second: "Hello"');
  assert.equal(arrivedText(undefined, "Hello"), 'Arrived: "Hello"', "a gnokey send has no wait to report");
});

test("a push arrival comes with the banner check, on its own line", () => {
  assert.equal(
    bannerCheck("push"),
    "No banner? Check this browser's notification settings in your system settings, and that Focus or Do Not Disturb is off.",
  );
  assert.equal(bannerCheck("live"), "", "the page showed it; no OS banner was due");
});

test("a copied command waits once while a send with its body is on time", () => {
  assert.equal(waitsFor([{ body: "Hello" }], "Hello"), true);
  assert.equal(waitsFor([{ body: "Hello", late: true }], "Hello"), false);
  assert.equal(waitsFor([{ body: "Bye" }], "Hello"), false);
});

test("a send's line names its transaction, and a copied command says what to do next", () => {
  assert.equal(sentText("adena", "aGFzaDE="), "Sent. Transaction aGFzaDE=.");
  assert.equal(sentText("adena", undefined), "Sent.");
  assert.equal(sentText("adena", ""), "Sent.");
  assert.equal(sentText("gnokey", undefined), "Copied. Run it in a terminal, then watch for your message below.");
});

test("a message's notification body fills the trigger's template and is cut as gnotifd delivers it", () => {
  assert.equal(bodyFor("{msg}", "Hello"), "Hello");
  assert.equal(bodyFor("You said: {msg}", "Hello"), "You said: Hello");
  assert.equal(bodyFor(undefined, "Hello"), "Hello", "before the trigger loads");
  assert.equal(bodyFor("{msg}", "🙂".repeat(100)), "🙂".repeat(63), "the preview shows what arrives");
});
