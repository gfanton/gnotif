import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const page = readFileSync(new URL("./index.html", import.meta.url), "utf8");

test("step 1's message line sits right under its buttons, above the notification state", () => {
  const disable = page.indexOf('id="disable"');
  const message = page.indexOf('id="message"');
  const state = page.indexOf('id="state"');
  assert.ok(disable !== -1 && message !== -1 && state !== -1);
  assert.ok(message > disable, "the message line follows the buttons");
  assert.ok(message < state, "the message line comes before the state line");
});

test("the address field is described by its own error line, between the field and its hint", () => {
  const input = page.match(/<input[^>]*\bid="address"[^>]*>/);
  assert.ok(input !== null, 'the page has an input with id="address"');
  assert.match(input[0], /aria-describedby="address-error address-hint"/);
  const error = page.indexOf('id="address-error"');
  assert.ok(error > page.indexOf('id="address-row"'), "the error line follows the field");
  assert.ok(error < page.indexOf('id="address-hint"'), "the error line comes before the hint");
});

test("the gnokey hint says to replace the key placeholder", () => {
  assert.ok(
    page.includes(
      "Replace <code>&lt;your key&gt;</code> with the name of your gnokey key for the address in step 1. The notification goes to whoever sends.",
    ),
  );
});

/** The position of needle in the page, failing the test when it is missing. */
function at(needle: string): number {
  const i = page.indexOf(needle);
  assert.ok(i !== -1, `the page has ${needle}`);
  return i;
}

/** The markup of the element that opens with start, through its closing tag. */
function block(start: string, close: string): string {
  const from = at(start);
  const to = page.indexOf(close, from);
  assert.ok(to !== -1, `${start} closes with ${close}`);
  return page.slice(from, to + close.length);
}

test("no label sits above a heading", () => {
  assert.ok(!page.includes('class="eyebrow"'));
});

test("step 1 presents notifications and live updates as two features, notifications first", () => {
  const order = [
    "<h2>Set up this browser</h2>",
    "Turn on notifications, live updates, or both. Either one is enough for step 2.",
    '<h3 id="push-head">Notifications</h3>',
    "This browser alerts you when your message arrives, even with the tab closed.",
    'id="enable"',
    'id="message"',
    'id="state"',
    '<h3 id="live-head">Live updates</h3>',
    "This page shows your message the moment it arrives, the way a dapp updates its own view. No permission prompt, and it works in any browser. While the page is open, it keeps a connection to the gnotif server and sends it your address.",
    'id="live"',
    'id="live-message"',
    'id="live-state"',
  ].map(at);
  assert.deepEqual(
    order,
    [...order].sort((a, b) => a - b),
    "in reading order",
  );
  assert.match(block('<p class="status" id="live-message"', "</p>"), /role="status"/);
});

test("the preview is a figure whose text screen readers read", () => {
  assert.ok(page.includes('<figure class="preview"'));
  assert.ok(page.includes('<figcaption class="caption">notification · this browser</figcaption>'));
  assert.ok(!block('<figure class="preview"', "</figure>").includes('role="img"'));
});

test("Enter runs each step's lead action", () => {
  const setup = block('<form id="setup"', "</form>");
  assert.match(setup, /<input type="text" id="address"/);
  assert.match(setup, /<button type="submit" class="button primary" id="enable"/);
  const compose = block('<form id="compose"', "</form>");
  assert.match(compose, /<input type="text" id="msg"/);
  assert.match(compose, /<button type="submit" class="button" id="send"/);
});

test("a wrong Adena account is caught before signing, so no line warns after it", () => {
  assert.ok(!page.includes('id="mismatch"'));
});

test("the faucet opens in a new tab, since the visitor comes back to send", () => {
  assert.ok(page.includes('<a href="https://faucet.gno.land" target="_blank" rel="noopener">Get test GNOT ↗</a>'));
});

test("the footer labels the server it prints", () => {
  assert.ok(page.includes('<span>gnotif server: <span class="mono" id="server"></span></span>'));
});

test("the chain note sits in the page's banner, with the nav", () => {
  const banner = block("<header>", "</header>");
  assert.ok(banner.includes('<div class="nav">'));
  assert.ok(banner.includes('<p class="chain" role="note">'));
});

test("the faucet link sits by the cost of a send, where test GNOT is needed, not in the banner", () => {
  assert.ok(!block("<header>", "</header>").includes("faucet.gno.land"));
  assert.ok(
    block('<form id="compose"', "</form>").includes(
      '<p class="hint" id="cost-hint">A send costs about 0.02 test GNOT from this address. <a href="https://faucet.gno.land" target="_blank" rel="noopener">Get test GNOT ↗</a></p>',
    ),
  );
});

test("links that leave the demo open a new tab and say so, since leaving drops a live connection and a waiting send", () => {
  for (const nav of [block('<nav aria-label="Site">', "</nav>"), block('<nav aria-label="Footer">', "</nav>")]) {
    const links = nav.match(/<a [^>]*>[^<]*<\/a>/g) ?? [];
    assert.ok(links.length >= 2, "each nav has its links");
    for (const link of links) {
      assert.match(link, /target="_blank" rel="noopener">[^<]* ↗<\/a>$/, link);
    }
  }
});

test("step 2's heading does not repeat the page's title", () => {
  assert.ok(block('<li class="step compose-step">', "</form>").includes("<h2>Send a message</h2>"));
});

test("the intro says what the echo realm does", () => {
  assert.ok(
    page.includes(
      '<p class="lede">One message, one on-chain event, one browser notification. Set up this browser, then send yourself a message through the echo realm, a small gno.land contract that emits each message as an event addressed to its sender.</p>',
    ),
  );
});

test("the address hint says a new Adena needs a reload", () => {
  assert.ok(
    page.includes(
      '<span id="get-adena" hidden> With Adena, connect it to fill this in: <a href="https://www.adena.app" target="_blank" rel="noopener">Get Adena ↗</a>, then reload this page.</span>',
    ),
  );
});

test("the gnokey path links to installing gnokey and making a key", () => {
  assert.ok(
    block('<details class="cli reveal" id="cli">', "</details>").includes(
      'No gnokey yet? <a href="https://docs.gno.land/users/interact-with-gnokey/" target="_blank" rel="noopener">Install it and make a key ↗</a>',
    ),
  );
});

test("each code reveal has a Copy button for its code, described by the code's label", () => {
  for (const code of ["code-js", "code-gno"]) {
    const head = page.slice(page.lastIndexOf('<div class="code-head">', at(`id="${code}"`)), at(`id="${code}"`));
    assert.match(head, new RegExp(`<span id="${code}-label">[^<]+</span>`));
    assert.match(
      head,
      new RegExp(
        `<button type="button" class="button small" data-copy="${code}" aria-describedby="${code}-label">Copy</button>`,
      ),
    );
  }
});

test("the wallet's messages sit under the address field, apart from the notifications line", () => {
  const field = block('<div class="field">', '<div class="channel">');
  assert.match(field, /<p class="status" id="wallet-message" role="status" aria-live="polite"><\/p>/);
  assert.ok(at('id="wallet-message"') < at('id="message"'));
});

test("a failure that stops the whole demo shows above the steps", () => {
  const page = at('<p class="status error" id="page-message" role="alert"></p>');
  assert.ok(page > at('<div class="intro">') && page < at('<ol class="steps">'));
});

test("an address error is announced when it appears", () => {
  assert.match(block('<p class="status error" id="address-error"', "</p>"), /aria-live="polite"/);
});

test("the notifications promise can hide where the browser cannot keep it", () => {
  assert.ok(
    page.includes(
      '<p class="hint" id="push-hint">This browser alerts you when your message arrives, even with the tab closed.</p>',
    ),
  );
});

test("without Adena, step 2 leads with a full-size copy button", () => {
  const compose = block('<form id="compose"', "</form>");
  assert.match(compose, /<button type="button" class="button" id="copy-lead" hidden>Copy the gnokey command<\/button>/);
  assert.ok(compose.includes('<p class="hint" id="cost-hint">A send costs about 0.02 test GNOT from this address.'));
});

test("on a phone, the gnokey path says where to run the command", () => {
  const cli = block('<details class="cli reveal" id="cli">', "</details>");
  assert.ok(
    cli.includes(
      '<p class="hint phone-only">On a phone? Run the command on a computer that has your key; this browser gets the notification.</p>',
    ),
  );
});

test("the banner check has its own quiet line under the send line", () => {
  assert.ok(at('<p class="status" id="banner-check" role="status"></p>') > at('id="sent"'));
});

test("the footer wordmark reads as one word", () => {
  assert.ok(page.includes('<span class="wordmark small" role="img" aria-label="gnotif">'));
});

test("step 2 previews the next notification, still, between the message and the send controls", () => {
  const compose = block('<li class="step compose-step">', "</li>");
  const preview = compose.indexOf('<figure class="preview"');
  assert.ok(preview > compose.indexOf('id="msg"'), "after the message field");
  assert.ok(preview < compose.indexOf('id="cost-hint"'), "before the cost line and the send controls");
  assert.ok(!page.includes('class="stage"') && !page.includes("sig-wire"), "no stage draws the send");
});
