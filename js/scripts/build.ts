// Builds the package into dist/.

import { execFileSync } from "node:child_process";
import { copyFileSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { build } from "esbuild";

import { valibotBanner } from "./banner.ts";

const root = join(import.meta.dirname, "..");
const dist = join(root, "dist");

// The package ships only gnotif.d.ts, so it must import none of the other
// declarations tsc writes. tsc runs first, so a type error leaves dist/ as it was.
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

  rmSync(dist, { recursive: true, force: true });

  await build({
    entryPoints: [join(root, "src", "gnotif.ts")],
    outfile: join(dist, "gnotif.js"),
    bundle: true,
    format: "esm",
    target: "es2022",
    banner: { js: valibotBanner },
  });

  // A dapp copies sw.js as a classic script, so it must hold no import or export.
  await build({
    entryPoints: [join(root, "src", "sw", "sw.ts")],
    outfile: join(dist, "sw.js"),
    bundle: true,
    format: "iife",
    target: "es2022",
    banner: {
      js: `/*! gnotif service worker. A dapp serves a copy of this file from its own\n * origin and registers it as sw.js?server=<gnotif server URL>, plus an\n * optional &icon=<image URL> that every notification shows. */\n${valibotBanner}`,
    },
  });

  copyFileSync(join(declarations, "gnotif.d.ts"), join(dist, "gnotif.d.ts"));
} finally {
  rmSync(declarations, { recursive: true, force: true });
}
