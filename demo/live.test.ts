import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const page = readFileSync(new URL("./index.html", import.meta.url), "utf8");

test("the live log follows the live switch's state line, in step 1", () => {
  const state = page.indexOf('id="live-state"');
  const log = page.indexOf('id="arrivals"');
  const step2 = page.indexOf("Step 2 of 2");
  assert.ok(state !== -1 && step2 !== -1);
  assert.ok(log > state, "the log comes after the live state line");
  assert.ok(log < step2, "the log comes before step 2");
});

test("the log's heading names no channel, since pushes list there with live updates off", () => {
  const head = page.match(/id="arrivals-head">([^<]*)</)?.[1];
  assert.ok(head !== undefined, 'the page has an element with id="arrivals-head"');
  assert.doesNotMatch(head, /live/i);
});

test("the live log is announced as messages arrive", () => {
  const tag = page.match(/<[^>]*\bid="arrivals"[^>]*>/);
  assert.ok(tag !== null, 'the page has an element with id="arrivals"');
  assert.ok(tag[0].includes('role="log"'), tag[0]);
});
