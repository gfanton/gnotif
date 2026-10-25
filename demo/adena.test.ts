import { test } from "node:test";
import assert from "node:assert/strict";
import { type Adena, AdenaError, connect, findAdena, sendEcho, shortAddress, walletLabel } from "./adena.ts";
import { callMessage, gas } from "./echo-tx.ts";

const chain = { id: "onyx-1", name: "Gno.land onyx testnet", rpc: "https://rpc.onyx.testnets.gno.land:443" };
const pkgPath = "gno.land/r/dev/echo/v1";
const address = "g1zrjyzatudmadgqu39d5wfamhdvf0adwlxkz25d";

const ok = (type: string, data: unknown) => ({ code: 0, status: "success", type, message: type, data });
// Adena's `message` is a fixed description of the type; the details, such as
// the chain's reason for a failed transaction, are in `data`.
const fail = (type: string, data: unknown = {}, message = `Adena's description of ${type}.`) => ({
  code: 4000,
  status: "failure",
  type,
  message,
  data,
});

type Method = keyof Adena;
type Options = { on?: string; known?: string[]; account?: string; answers?: Partial<Record<Method, unknown>> };
type FakeAdena = Adena & { calls: [Method, ...unknown[]][] };

// fakeAdena answers like the extension's window.adena and records every call.
// `on` is the wallet's current chain, `known` the chains it can switch to without
// adding, `account` the chain GetAccount reports ("current" follows the switch),
// and `answers` overrides a method's response.
function fakeAdena({ on = chain.id, known = [], account = chain.id, answers = {} }: Options = {}): FakeAdena {
  const calls: [Method, ...unknown[]][] = [];
  const added = new Set([on, ...known]);
  let current = on;
  return {
    calls,
    AddEstablish: async (name) => {
      calls.push(["AddEstablish", name]);
      return answers.AddEstablish ?? ok("CONNECTION_SUCCESS", { name });
    },
    GetNetwork: async () => {
      calls.push(["GetNetwork"]);
      return answers.GetNetwork ?? ok("GET_NETWORK_SUCCESS", { chainId: current, networkName: current, rpcUrl: "" });
    },
    SwitchNetwork: async (id) => {
      calls.push(["SwitchNetwork", id]);
      if (answers.SwitchNetwork !== undefined) return answers.SwitchNetwork;
      if (!added.has(id)) return fail("UNADDED_NETWORK");
      current = id;
      return ok("SWITCH_NETWORK_SUCCESS", { chainId: id });
    },
    AddNetwork: async (network) => {
      calls.push(["AddNetwork", network]);
      if (answers.AddNetwork !== undefined) return answers.AddNetwork;
      added.add(network.chainId);
      return ok("ADD_NETWORK_SUCCESS", network);
    },
    GetAccount: async () => {
      calls.push(["GetAccount"]);
      return answers.GetAccount ?? ok("GET_ACCOUNT", { address, chainId: account === "current" ? current : account });
    },
    DoContract: async (params) => {
      calls.push(["DoContract", params]);
      return answers.DoContract ?? ok("TRANSACTION_SUCCESS", { hash: "abc123", height: "7" });
    },
  };
}

const names = (wallet: FakeAdena): Method[] => wallet.calls.map((c) => c[0]);

test("connect establishes, checks the network and returns the account address", async () => {
  const wallet = fakeAdena();
  assert.equal(await connect(wallet, chain), address);
  assert.deepEqual(names(wallet), ["AddEstablish", "GetNetwork", "GetAccount"]);
  assert.equal(typeof wallet.calls[0]?.[1], "string", "AddEstablish names the site");
});

test("connect treats ALREADY_CONNECTED as connected", async () => {
  const wallet = fakeAdena({ answers: { AddEstablish: fail("ALREADY_CONNECTED") } });
  assert.equal(await connect(wallet, chain), address);
});

test("connect switches when the wallet is on another chain", async () => {
  const wallet = fakeAdena({ on: "dev", known: [chain.id], account: "current" });
  assert.equal(await connect(wallet, chain), address);
  assert.deepEqual(names(wallet), ["AddEstablish", "GetNetwork", "SwitchNetwork", "GetAccount"]);
  assert.equal(wallet.calls[2]?.[1], chain.id);
});

test("connect adds the network and switches again on UNADDED_NETWORK", async () => {
  const wallet = fakeAdena({ on: "dev", account: "current" });
  assert.equal(await connect(wallet, chain), address);
  assert.deepEqual(names(wallet), [
    "AddEstablish",
    "GetNetwork",
    "SwitchNetwork",
    "AddNetwork",
    "SwitchNetwork",
    "GetAccount",
  ]);
  assert.deepEqual(wallet.calls[3]?.[1], { chainId: chain.id, chainName: chain.name, rpcUrl: chain.rpc });
});

test("connect accepts REDUNDANT_CHANGE_REQUEST from the switch", async () => {
  const wallet = fakeAdena({ on: "dev", answers: { SwitchNetwork: fail("REDUNDANT_CHANGE_REQUEST") } });
  assert.equal(await connect(wallet, chain), address);
});

test("connect refuses when GetAccount reports another chain", async () => {
  const wallet = fakeAdena({ account: "dev" });
  await assert.rejects(connect(wallet, chain), (err) => {
    assert.ok(err instanceof AdenaError);
    assert.equal(err.type, "WRONG_CHAIN");
    assert.match(err.message, /onyx-1/);
    return true;
  });
});

test("sendEcho runs the sequence and calls Echo with the message", async () => {
  const wallet = fakeAdena();
  assert.deepEqual(await sendEcho(wallet, chain, pkgPath, "hello", address), { caller: address, hash: "abc123" });
  assert.deepEqual(names(wallet), ["AddEstablish", "GetNetwork", "GetAccount", "DoContract"]);
  assert.deepEqual(
    wallet.calls[3]?.[1],
    {
      messages: [callMessage(address, pkgPath, "hello")],
      gasFee: gas.fee,
      gasWanted: gas.wanted,
      memo: "",
      networkInfo: { chainId: chain.id, rpcUrl: chain.rpc },
    },
    "networkInfo pins the chain Adena signs for",
  );
});

test("sendEcho refuses to sign from an account step 1 does not listen for", async () => {
  const listening = "g13rqvks880v0hmlfepcj0kkudq0js38d3pc5k6a";
  const wallet = fakeAdena();
  await assert.rejects(sendEcho(wallet, chain, pkgPath, "hello", listening), {
    name: "AdenaError",
    type: "WRONG_ACCOUNT",
    message:
      "Adena's active account is g1zrjy…z25d, but step 1 listens for g13rqv…5k6a. Switch Adena to that account, then send again.",
  });
  assert.ok(!names(wallet).includes("DoContract"), "the wrong account costs no transaction");
});

test("sendEcho never calls DoContract when the account is on another chain", async () => {
  const wallet = fakeAdena({ account: "dev" });
  await assert.rejects(sendEcho(wallet, chain, pkgPath, "hello", address), AdenaError);
  assert.ok(!names(wallet).includes("DoContract"));
});

// Each refusal names the wallet options, the failure type sendEcho throws, and
// the calls made up to the refusal.
const refusals: Record<string, [Options, string, Method[]]> = {
  "GetNetwork fails": [
    { answers: { GetNetwork: fail("WALLET_LOCKED") } },
    "WALLET_LOCKED",
    ["AddEstablish", "GetNetwork"],
  ],
  "GetAccount fails": [
    { answers: { GetAccount: fail("NO_ACCOUNT") } },
    "NO_ACCOUNT",
    ["AddEstablish", "GetNetwork", "GetAccount"],
  ],
  "the visitor rejects the switch": [
    { on: "dev", answers: { SwitchNetwork: fail("SWITCH_NETWORK_REJECTED") } },
    "SWITCH_NETWORK_REJECTED",
    ["AddEstablish", "GetNetwork", "SwitchNetwork"],
  ],
  // AddNetwork answers success without adding the chain, so the second switch still finds it unadded.
  "the switch after AddNetwork fails": [
    { on: "dev", answers: { AddNetwork: ok("ADD_NETWORK_SUCCESS", {}) } },
    "UNADDED_NETWORK",
    ["AddEstablish", "GetNetwork", "SwitchNetwork", "AddNetwork", "SwitchNetwork"],
  ],
};
for (const [name, [options, type, calls]] of Object.entries(refusals)) {
  test(`sendEcho never calls DoContract when ${name}`, async () => {
    const wallet = fakeAdena(options);
    await assert.rejects(sendEcho(wallet, chain, pkgPath, "hello", address), (err) => {
      assert.ok(err instanceof AdenaError);
      assert.equal(err.type, type);
      return true;
    });
    assert.deepEqual(names(wallet), calls);
  });
}

test("sendEcho maps each failure type to its own message", async () => {
  const cases = {
    TRANSACTION_REJECTED: /rejected/i,
    WALLET_LOCKED: /unlock/i,
    NETWORK_TIMEOUT: /reach|try again/i,
    TRANSACTION_FAILED: /failed/i,
    ACCOUNT_MISMATCH: /account/i,
  };
  const messages = new Set<string>();
  for (const [type, pattern] of Object.entries(cases)) {
    const wallet = fakeAdena({ answers: { DoContract: fail(type) } });
    await assert.rejects(sendEcho(wallet, chain, pkgPath, "hello", address), (err) => {
      assert.ok(err instanceof AdenaError, type);
      assert.equal(err.type, type);
      assert.match(err.message, pattern, type);
      messages.add(err.message);
      return true;
    });
  }
  assert.equal(messages.size, Object.keys(cases).length, "each type has its own message");
});

test("a rejection stands alone, a chain failure carries the chain's reason", async () => {
  const rejected = fakeAdena({ answers: { DoContract: fail("TRANSACTION_REJECTED") } });
  await assert.rejects(sendEcho(rejected, chain, pkgPath, "hello", address), {
    message: "You rejected the transaction in Adena.",
  });
  const failed = fakeAdena({
    answers: { DoContract: fail("TRANSACTION_FAILED", { hash: "abc123", error: "out of gas" }) },
  });
  await assert.rejects(sendEcho(failed, chain, pkgPath, "hello", address), {
    message: "The transaction failed on the chain: out of gas.",
  });
});

test("a chain reason that ends in a period does not double it", async () => {
  const wallet = fakeAdena({
    answers: { DoContract: fail("TRANSACTION_FAILED", { hash: "abc123", error: "insufficient funds." }) },
  });
  await assert.rejects(sendEcho(wallet, chain, pkgPath, "hello", address), {
    message: "The transaction failed on the chain: insufficient funds.",
  });
});

test("a chain failure without a reason says only that it failed", async () => {
  for (const data of [{ hash: "abc123", error: null }, {}]) {
    const wallet = fakeAdena({ answers: { DoContract: fail("TRANSACTION_FAILED", data) } });
    await assert.rejects(sendEcho(wallet, chain, pkgPath, "hello", address), {
      message: "The transaction failed on the chain.",
    });
  }
});

test("an unknown failure type keeps Adena's own message", async () => {
  const wallet = fakeAdena({
    answers: { AddEstablish: fail("NOT_CONNECTED", {}, "A connection has not been established.") },
  });
  await assert.rejects(connect(wallet, chain), (err) => {
    assert.ok(err instanceof AdenaError);
    assert.equal(err.type, "NOT_CONNECTED");
    assert.equal(err.message, "Adena answered NOT_CONNECTED: A connection has not been established.");
    return true;
  });
});

test("a failed AddNetwork stops the sequence", async () => {
  const wallet = fakeAdena({ on: "dev", answers: { AddNetwork: fail("ADD_NETWORK_REJECTED") } });
  await assert.rejects(
    connect(wallet, chain),
    (err) => err instanceof AdenaError && err.type === "ADD_NETWORK_REJECTED",
  );
  assert.ok(!names(wallet).includes("GetAccount"));
});

/** A window whose document is in readyState, with a load event to fire. */
function fakeWindow(readyState: string, adena: unknown) {
  const listeners: (() => void)[] = [];
  return {
    adena,
    document: { readyState },
    addEventListener(type: string, fn: () => void): void {
      if (type === "load") {
        listeners.push(fn);
      }
    },
    load(): void {
      listeners.forEach((fn) => fn());
    },
  };
}

test("findAdena answers at once when Adena is already there", async () => {
  const wallet = fakeAdena();
  assert.equal(await findAdena(fakeWindow("loading", wallet)), wallet);
});

test("findAdena waits for the page to load, as Adena injects itself late", async () => {
  const win = fakeWindow("interactive", undefined);
  const found = findAdena(win);
  const wallet = fakeAdena();
  win.adena = wallet;
  win.load();
  assert.equal(await found, wallet);
});

test("findAdena answers undefined on a loaded page without Adena", async () => {
  assert.equal(await findAdena(fakeWindow("complete", undefined)), undefined);
});

test("findAdena answers undefined when window.adena lacks a method the demo calls", async () => {
  for (const adena of [{ ...fakeAdena(), SwitchNetwork: undefined }, {}, "adena"]) {
    assert.equal(await findAdena(fakeWindow("complete", adena)), undefined, JSON.stringify(adena));
  }
});

test("walletLabel says what the Adena button does in each state", () => {
  assert.equal(walletLabel("idle"), "Connect Adena");
  assert.equal(walletLabel("pending"), "Approve in Adena…");
  assert.equal(walletLabel("connected", address), "Adena · g1zrjy…z25d");
});

test("shortAddress keeps the start and the end of an address", () => {
  assert.equal(shortAddress(address), "g1zrjy…z25d");
});
