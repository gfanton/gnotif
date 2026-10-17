import test from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

import * as v from "valibot";

const root = join(import.meta.dirname, "..");

const ManifestSchema = v.object({
  exports: v.object({
    ".": v.object({ types: v.string(), default: v.string() }),
    "./sw.js": v.string(),
  }),
});

const PackSchema = v.tuple([v.object({ files: v.array(v.object({ path: v.string() })) })]);

test("every exports target exists after the build", () => {
  const manifest = v.parse(ManifestSchema, JSON.parse(readFileSync(join(root, "package.json"), "utf8")));
  const targets = [manifest.exports["."].types, manifest.exports["."].default, manifest.exports["./sw.js"]];
  for (const target of targets) {
    assert.ok(existsSync(join(root, target)), target);
  }
});

test("the packed package holds only the manifest, the docs and dist", () => {
  const out = execFileSync("npm", ["pack", "--dry-run", "--json"], { cwd: root, encoding: "utf8" });
  const [pack] = v.parse(PackSchema, JSON.parse(out));
  const paths = pack.files.map((f) => f.path).sort();
  assert.deepEqual(paths, ["LICENSE", "README.md", "dist/gnotif.d.ts", "dist/gnotif.js", "dist/sw.js", "package.json"]);
});

test("gnotif.d.ts imports nothing", () => {
  assert.doesNotMatch(readFileSync(join(root, "dist", "gnotif.d.ts"), "utf8"), /\bimport\b/);
});

test("a strict consumer compiles against the published types", () => {
  execFileSync("tsc", ["-p", join(root, "test", "consumer", "tsconfig.json")], { stdio: "inherit" });
});
