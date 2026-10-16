import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const read = (path: string): string => readFileSync(new URL(path, import.meta.url), "utf8");
const page = read("./index.html");

/** The text of the <code id="..."> block in the page, with its entities decoded. */
function snippet(id: string): string {
  const m = page.match(new RegExp(`<code id="${id}">([\\s\\S]*?)</code>`))?.[1];
  assert.ok(m !== undefined, `the page has a <code id="${id}"> block`);
  return m
    .replace(/<[^>]+>/g, "")
    .replaceAll("&lt;", "<")
    .replaceAll("&gt;", ">")
    .replaceAll("&quot;", '"')
    .replaceAll("&#34;", '"')
    .replaceAll("&#39;", "'")
    .replaceAll("&amp;", "&");
}

const lines = (text: string): string[] =>
  text
    .split("\n")
    .map((l) => l.trim())
    .filter((l) => l !== "");

test("the Gno snippet is lines of the echo realm", () => {
  const realm = new Set(lines(read("./gno.land/r/echo/v1/echo.gno")));
  const code = lines(snippet("code-gno"));
  assert.ok(code.length > 0);
  for (const line of code) {
    assert.ok(realm.has(line), `echo.gno has the line: ${line}`);
  }
});

test("the JavaScript snippet makes the calls the page makes", () => {
  const code = snippet("code-js");
  const app = read("./app.ts");
  assert.ok(
    code.includes('new Gnotif({ network: "onyx", icon: "/icon.png" })'),
    "the snippet names its network and icon",
  );
  assert.ok(app.includes('icon: "/icon.png"'), "app.ts passes the snippet's icon");
  for (const call of [".triggers(", ".enable()", ".setOptins(", '.on("notification"', ".connect()", ".disconnect()"]) {
    assert.ok(code.includes(call), `the snippet calls ${call}`);
    assert.ok(app.includes(call), `app.ts calls ${call}`);
  }
});
