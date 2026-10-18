// Builds the demo into dist/ for the config.ts entry DEMO_NETWORK names,
// local when it is unset or empty. An unknown name stops it before it writes.

import { copyFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";

import { build } from "esbuild";

import { configFor } from "./config.ts";

const network = process.env.DEMO_NETWORK;
const name = network === undefined || network === "" ? "local" : network;
configFor(name);

const root = import.meta.dirname;
const dist = join(root, "dist");

rmSync(dist, { recursive: true, force: true });

await build({
  entryPoints: [join(root, "app.ts")],
  outfile: join(dist, "app.js"),
  bundle: true,
  format: "esm",
  define: { DEMO_NETWORK: JSON.stringify(name) },
  // Skips the paths of demo/tsconfig.json, so the bundle takes the built
  // package through its exports, as a dapp does.
  tsconfig: join(root, "..", "tsconfig.base.json"),
});

copyFileSync(join(root, "index.html"), join(dist, "index.html"));
copyFileSync(join(root, "icon.png"), join(dist, "icon.png"));
copyFileSync(fileURLToPath(import.meta.resolve("gnotif/sw.js")), join(dist, "sw.js"));
