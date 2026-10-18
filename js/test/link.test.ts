import test from "node:test";
import assert from "node:assert/strict";

import { safeLink } from "../src/link.ts";

const ORIGIN = "https://dapp.example";

test("safeLink keeps links on the worker's origin and falls back to its scope", () => {
  for (const scope of [`${ORIGIN}/`, `${ORIGIN}/app/`]) {
    const cases: [string, string][] = [
      ["/?game=7", `${ORIGIN}/?game=7`],
      ["/app/?game=7", `${ORIGIN}/app/?game=7`],
      ["./", scope],
      ["//evil.com", scope],
      ["/\\evil.com", scope],
      ["https://evil.com", scope],
      ["javascript:alert(1)", scope],
    ];
    for (const [link, want] of cases) {
      assert.equal(safeLink(link, scope), want, `${link} under ${scope}`);
    }
  }
});
