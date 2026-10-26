import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const page = readFileSync(new URL("./index.html", import.meta.url), "utf8");

test("the log of sends and arrivals sits in step 2, under the send line", () => {
  const from = page.indexOf('<li class="step compose-step">');
  const step = page.slice(from, page.indexOf("</li>", from));
  const log = step.indexOf('id="arrivals"');
  assert.ok(log !== -1, "the log is in step 2");
  assert.ok(log > step.indexOf('id="banner-check"'), "after the send line and the banner check");
  assert.ok(page.includes('<h3 id="arrivals-head">Your messages</h3>'));
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
