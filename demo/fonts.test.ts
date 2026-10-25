import test from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";

const read = (path: string): string => readFileSync(new URL(path, import.meta.url), "utf8");

test("the page serves its own fonts, the headline's face first", () => {
  const page = read("./index.html");
  assert.ok(!page.includes("fonts.googleapis.com"));
  assert.ok(!page.includes("fonts.gstatic.com"));
  assert.ok(
    page.includes(
      '<link rel="preload" href="./fonts/ibm-plex-mono-500-latin.woff2" as="font" type="font/woff2" crossorigin>',
    ),
  );
});

test("each face is one of gnotif.xyz's font files, shown with the fallback until it loads", () => {
  const faces = read("./style.css").match(/@font-face \{[^}]*\}/g) ?? [];
  assert.equal(faces.length, 12, "two families, three weights, latin and latin-ext");
  for (const face of faces) {
    assert.match(face, /font-display: swap;/);
    const file = face.match(/url\("\.\/fonts\/([^"]+\.woff2)"\)/)?.[1];
    assert.ok(file !== undefined, face);
    assert.ok(existsSync(new URL(`../site/static/fonts/${file}`, import.meta.url)), `site/static/fonts has ${file}`);
    assert.ok(existsSync(new URL(`./dist/fonts/${file}`, import.meta.url)), `the build copies ${file}`);
  }
  assert.ok(existsSync(new URL("./dist/fonts/OFL.txt", import.meta.url)), "the fonts ship with their license");
});
