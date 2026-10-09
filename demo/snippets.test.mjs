import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const read = (path) => readFileSync(new URL(path, import.meta.url), "utf8");
const page = read("./index.html");

/** The text of the <code id="..."> block in the page, with its entities decoded. */
function snippet(id) {
  const m = page.match(new RegExp(`<code id="${id}">([\\s\\S]*?)</code>`));
  assert.ok(m, `the page has a <code id="${id}"> block`);
  return m[1].replace(/<[^>]+>/g, "").replaceAll("&lt;", "<").replaceAll("&gt;", ">").replaceAll("&quot;", '"').replaceAll("&#34;", '"').replaceAll("&#39;", "'").replaceAll("&amp;", "&");
}

const lines = (text) => text.split("\n").map((l) => l.trim()).filter((l) => l !== "");

test("the Gno snippet is lines of the echo realm", () => {
  const realm = new Set(lines(read("./gno.land/r/echo/v0/echo.gno")));
  const code = lines(snippet("code-gno"));
  assert.ok(code.length > 0);
  for (const line of code) {
    assert.ok(realm.has(line), `echo.gno has the line: ${line}`);
  }
});

test("the JavaScript snippet makes the calls the page makes", () => {
  const code = snippet("code-js");
  const app = read("./app.mjs");
  assert.ok(code.includes('new Gnotif({ network: "onyx", icon: "/icon.png" })'), "the snippet names its network and icon");
  assert.ok(app.includes('icon: "/icon.png"'), "app.mjs passes the snippet's icon");
  for (const call of [".triggers(", ".enable()", ".setOptins("]) {
    assert.ok(code.includes(call), `the snippet calls ${call}`);
    assert.ok(app.includes(call), `app.mjs calls ${call}`);
  }
});
