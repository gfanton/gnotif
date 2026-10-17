// Builds the package into dist/.

import { execFileSync } from "node:child_process";
import { copyFileSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { build } from "esbuild";

const root = join(import.meta.dirname, "..");
const dist = join(root, "dist");

rmSync(dist, { recursive: true, force: true });

await build({
  entryPoints: [join(root, "src", "gnotif.ts")],
  outfile: join(dist, "gnotif.js"),
  bundle: true,
  format: "esm",
  target: "es2022",
});

// A dapp copies sw.js as a classic script, so it must hold no import or export.
await build({
  entryPoints: [join(root, "src", "sw", "sw.ts")],
  outfile: join(dist, "sw.js"),
  bundle: true,
  format: "iife",
  target: "es2022",
});

// The package ships only gnotif.d.ts, so it must import none of the other
// declarations tsc writes.
const declarations = mkdtempSync(join(tmpdir(), "gnotif-types-"));
try {
  execFileSync(
    "tsc",
    [
      "-p",
      join(root, "tsconfig.json"),
      "--noEmit",
      "false",
      "--declaration",
      "--emitDeclarationOnly",
      "--rootDir",
      join(root, "src"),
      "--outDir",
      declarations,
    ],
    { stdio: "inherit" },
  );
  copyFileSync(join(declarations, "gnotif.d.ts"), join(dist, "gnotif.d.ts"));
} finally {
  rmSync(declarations, { recursive: true, force: true });
}
