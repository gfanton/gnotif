import { test } from "node:test";
import assert from "node:assert/strict";
import { callMessage, gas, gnokeyCommand, shellQuote } from "./echo-tx.mjs";

const chain = { id: "onyx-1", name: "Gno.land onyx testnet", rpc: "https://rpc.onyx.testnets.gno.land:443" };
const pkgPath = "gno.land/r/dev/echo/v0";

test("shellQuote wraps the text in single quotes", () => {
  assert.equal(shellQuote("hello"), "'hello'");
  assert.equal(shellQuote(""), "''");
});

test("shellQuote keeps spaces, $ and newlines literal", () => {
  assert.equal(shellQuote("hi there $HOME"), "'hi there $HOME'");
  assert.equal(shellQuote("two\nlines"), "'two\nlines'");
  assert.equal(shellQuote("a\\b"), "'a\\b'");
});

test("shellQuote escapes a single quote as '\\''", () => {
  assert.equal(shellQuote("it's"), "'it'\\''s'");
  assert.equal(shellQuote("''"), "''\\'''\\'''");
});

test("gnokeyCommand builds a call to Echo on the configured chain", () => {
  assert.equal(
    gnokeyCommand(chain, pkgPath, "hello"),
    "gnokey maketx call -pkgpath gno.land/r/dev/echo/v0 -func Echo -args 'hello'"
      + ` -gas-wanted ${gas.wanted} -gas-fee ${gas.fee}ugnot`
      + " -chainid onyx-1 -remote https://rpc.onyx.testnets.gno.land:443 <your key>",
  );
});

test("gnokeyCommand quotes the message", () => {
  const cmd = gnokeyCommand(chain, pkgPath, "it's $1, ok?");
  assert.match(cmd, /-args 'it'\\''s \$1, ok\?' -gas-wanted/);
});

test("gas values are positive integers", () => {
  assert.ok(Number.isInteger(gas.wanted) && gas.wanted > 0);
  assert.ok(Number.isInteger(gas.fee) && gas.fee > 0);
});

test("callMessage is a /vm.m_call to Echo from the caller", () => {
  assert.deepEqual(callMessage("g1caller", pkgPath, "hi"), {
    type: "/vm.m_call",
    value: { caller: "g1caller", send: "", max_deposit: "", pkg_path: pkgPath, func: "Echo", args: ["hi"] },
  });
});
