import { test } from "node:test";
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { join } from "node:path";

import { configFor } from "./config.ts";

function build(network: string): { status: number | null; stderr: string } {
  return spawnSync(process.execPath, [join(import.meta.dirname, "build.ts")], {
    cwd: import.meta.dirname,
    env: { ...process.env, DEMO_NETWORK: network },
    encoding: "utf8",
  });
}

test("configFor returns the local and onyx entries and throws on another name", () => {
  assert.equal(configFor("local").echo, "gno.land/r/dev/echo/v1");
  assert.equal(configFor("onyx").chain.id, "onyx-1");
  assert.throws(() => configFor("mainnet"), /unknown network "mainnet"/);
});

test("the build refuses an unknown DEMO_NETWORK", () => {
  const r = build("mainnet");
  assert.notEqual(r.status, 0);
  assert.match(r.stderr, /unknown network "mainnet"/);
});

test("an empty DEMO_NETWORK builds the local config", () => {
  const r = build("");
  assert.equal(r.status, 0, r.stderr);
  assert.match(readFileSync(new URL("./dist/app.js", import.meta.url), "utf8"), /configFor\("local"\)/);
});
