import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const page = readFileSync(new URL("./index.html", import.meta.url), "utf8");

test("the page carries link-preview tags with an absolute image URL", () => {
  for (const tag of [
    '<meta property="og:url" content="https://demo.gnotif.xyz/">',
    '<meta property="og:image" content="https://gnotif.xyz/og.png">',
    '<meta name="twitter:card" content="summary_large_image">',
  ]) {
    assert.ok(page.includes(tag), tag);
  }
  assert.match(page, /<meta property="og:title" content="[^"]+">/);
  assert.match(page, /<meta property="og:description" content="[^"]+">/);
});
