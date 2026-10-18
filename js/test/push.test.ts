import test from "node:test";
import assert from "node:assert/strict";

import { parsePayload } from "../src/push.ts";

const FALLBACK = { title: "New activity", body: "", link: "./" };

test("parsePayload returns the fields of a valid payload", () => {
  const p = parsePayload(JSON.stringify({ title: "Your turn", body: "Game 7", link: "/?game=7" }));
  assert.deepEqual({ ...p }, { title: "Your turn", body: "Game 7", link: "/?game=7" });
});

test("parsePayload falls back on malformed payloads", () => {
  for (const text of ["not json", '{"body":"x"}', '{"title":5}', '{"title":"","body":"x","link":"/"}']) {
    assert.deepEqual({ ...parsePayload(text) }, FALLBACK, text);
  }
});
