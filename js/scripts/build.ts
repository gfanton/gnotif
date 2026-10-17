// Builds the package into dist/. Paths are relative to js/, where pnpm runs
// the build script.

import { execFileSync } from "node:child_process";
import { copyFileSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { build } from "esbuild";

rmSync("dist", { recursive: true, force: true });

await build({
  entryPoints: ["src/gnotif.ts"],
  outfile: "dist/gnotif.js",
  bundle: true,
  format: "esm",
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
      "tsconfig.json",
      "--noEmit",
      "false",
      "--declaration",
      "--emitDeclarationOnly",
      "--rootDir",
      "src",
      "--outDir",
      declarations,
    ],
    { stdio: "inherit" },
  );
  copyFileSync(join(declarations, "gnotif.d.ts"), "dist/gnotif.d.ts");
} finally {
  rmSync(declarations, { recursive: true, force: true });
}
