// The Adena sequence, against the injected window.adena. The page branches on
// an answer's `status` and `type`, never on its numeric `code`, since the codes
// differ between the extension and its SDK.
import * as v from "valibot";

import { callMessage, gas } from "./echo-tx.ts";

const SITE = "gnotif demo";

/**
 * An answer from window.adena. `message` is Adena's fixed description of
 * `type`. `data` holds the method's result on success, and a failure's details
 * otherwise, such as the chain's reason for a failed transaction in `error`.
 */
const AnswerSchema = v.object({
  status: v.string(),
  type: v.string(),
  message: v.string(),
  data: v.optional(v.unknown()),
});

type Answer = v.InferOutput<typeof AnswerSchema>;

const FailureSchema = v.object({ error: v.string() });
const NetworkSchema = v.object({ chainId: v.string() });
const AccountSchema = v.object({ address: v.string(), chainId: v.string() });
const SentSchema = v.object({ hash: v.optional(v.string()) });

/** The window.adena methods the demo calls. Each answer is parsed where it is awaited. */
export type Adena = {
  AddEstablish: (site: string) => unknown;
  GetNetwork: () => unknown;
  SwitchNetwork: (chainId: string) => unknown;
  AddNetwork: (network: { chainId: string; chainName: string; rpcUrl: string }) => unknown;
  GetAccount: () => unknown;
  DoContract: (params: {
    messages: ReturnType<typeof callMessage>[];
    memo?: string;
    gasFee?: number;
    gasWanted?: number;
    networkInfo?: { chainId: string; rpcUrl: string };
  }) => unknown;
};

const AdenaSchema = v.looseObject({
  AddEstablish: v.function(),
  GetNetwork: v.function(),
  SwitchNetwork: v.function(),
  AddNetwork: v.function(),
  GetAccount: v.function(),
  DoContract: v.function(),
});

/** A failure Adena reported, with its `type` and a message for the visitor. */
export class AdenaError extends Error {
  type: string;

  /** @param type Adena's response type, WRONG_CHAIN or WRONG_ACCOUNT */
  constructor(type: string, message: string) {
    super(message);
    this.name = "AdenaError";
    this.type = type;
  }
}

/** @param reason the chain's reason for a failed transaction, as ": <reason>", or "" */
const messages = (chain: { name: string }, reason: string): Record<string, string | undefined> => ({
  TRANSACTION_REJECTED: "You rejected the transaction in Adena.",
  WALLET_LOCKED: "Adena is locked. Unlock it, then try again.",
  NETWORK_TIMEOUT: `Adena could not reach ${chain.name}. Try again in a moment.`,
  TRANSACTION_FAILED: `The transaction failed on the chain${reason}.`,
  ACCOUNT_MISMATCH: "Adena's active account changed. Connect again.",
  UNADDED_NETWORK: `Adena does not know the ${chain.name} network.`,
});

function parsed<T>(schema: v.GenericSchema<unknown, T>, data: unknown): T {
  const result = v.safeParse(schema, data);
  if (!result.success) {
    throw new Error("Adena sent an answer this page cannot read.");
  }
  return result.output;
}

/** failure is the AdenaError for a failed answer, with its message for the visitor. */
function failure(a: Answer, chain: { name: string }): AdenaError {
  const details = v.safeParse(FailureSchema, a.data);
  const error = details.success ? details.output.error.replace(/\.+$/, "") : "";
  const reason = error !== "" ? `: ${error}` : "";
  const known = messages(chain, reason)[a.type];
  const description = a.message === "" ? "." : `: ${a.message}`;
  return new AdenaError(a.type, known ?? `Adena answered ${a.type}${description}`);
}

/**
 * check returns the answer when Adena reports success, or one of the
 * accepted failure types, and throws an AdenaError otherwise.
 * @param accepted failure types that count as success here
 */
function check(answer: unknown, chain: { name: string }, accepted: string[] = []): Answer {
  const a = parsed(AnswerSchema, answer);
  if (a.status === "success" || accepted.includes(a.type)) {
    return a;
  }
  throw failure(a, chain);
}

/**
 * findAdena resolves to the injected Adena wallet, or undefined without one,
 * or when window.adena lacks a method the demo calls. The extension injects
 * window.adena after the page's scripts run, so a page still loading is
 * checked again once it has loaded.
 */
export function findAdena(win: {
  adena?: unknown;
  document: { readyState: string };
  addEventListener(type: "load", fn: () => void, options: { once: boolean }): void;
}): Promise<Adena | undefined> {
  const found = (): Adena | undefined => (v.is(AdenaSchema, win.adena) ? win.adena : undefined);
  if (win.adena !== undefined || win.document.readyState === "complete") {
    return Promise.resolve(found());
  }
  return new Promise((resolve) => win.addEventListener("load", () => resolve(found()), { once: true }));
}

/** @returns the address cut to its start and end, such as "g1zrjy…z25d" */
export function shortAddress(address: string): string {
  return `${address.slice(0, 6)}…${address.slice(-4)}`;
}

/**
 * walletLabel is the text of an Adena button: its action, the wait for the
 * Adena popup, or the connected account.
 * @param address the connected account, for "connected"
 */
export function walletLabel(state: "idle" | "pending" | "connected", address = ""): string {
  switch (state) {
    case "pending":
      return "Approve in Adena…";
    case "connected":
      return `Adena · ${shortAddress(address)}`;
    case "idle":
      return "Connect Adena";
  }
}

/**
 * connect establishes the site with Adena, moves it to chain, and returns the
 * active account's address. It refuses an account on another chain.
 * @param adena window.adena
 */
export async function connect(adena: Adena, chain: { id: string; name: string; rpc: string }): Promise<string> {
  check(await adena.AddEstablish(SITE), chain, ["ALREADY_CONNECTED"]);
  const network = parsed(NetworkSchema, check(await adena.GetNetwork(), chain).data);
  if (network.chainId !== chain.id) {
    let switched = parsed(AnswerSchema, await adena.SwitchNetwork(chain.id));
    if (switched.type === "UNADDED_NETWORK") {
      check(await adena.AddNetwork({ chainId: chain.id, chainName: chain.name, rpcUrl: chain.rpc }), chain);
      switched = parsed(AnswerSchema, await adena.SwitchNetwork(chain.id));
    }
    check(switched, chain, ["REDUNDANT_CHANGE_REQUEST"]);
  }
  const account = parsed(AccountSchema, check(await adena.GetAccount(), chain).data);
  if (account.chainId !== chain.id) {
    throw new AdenaError(
      "WRONG_CHAIN",
      `Adena's account is on ${account.chainId}, not ${chain.id}. Switch Adena to ${chain.name}, then try again.`,
    );
  }
  return account.address;
}

/**
 * sendEcho connects, then has Adena sign and broadcast Echo(msg) from the
 * active account. It refuses before signing when that account is not from,
 * since the notification goes to the caller.
 * @param adena window.adena
 * @param pkgPath the echo realm
 * @param from the address step 1 listens for
 */
export async function sendEcho(
  adena: Adena,
  chain: { id: string; name: string; rpc: string },
  pkgPath: string,
  msg: string,
  from: string,
): Promise<{ caller: string; hash: string | undefined }> {
  const caller = await connect(adena, chain);
  if (caller !== from) {
    throw new AdenaError(
      "WRONG_ACCOUNT",
      `Adena's active account is ${shortAddress(caller)}, but step 1 listens for ${shortAddress(from)}. Switch Adena to that account, then send again.`,
    );
  }
  const sent = parsed(
    AnswerSchema,
    await adena.DoContract({
      messages: [callMessage(caller, pkgPath, msg)],
      gasFee: gas.fee,
      gasWanted: gas.wanted,
      memo: "",
      networkInfo: { chainId: chain.id, rpcUrl: chain.rpc },
    }),
  );
  if (sent.type !== "TRANSACTION_SUCCESS") {
    throw failure(sent, chain);
  }
  const data = v.safeParse(SentSchema, sent.data);
  return { caller, hash: data.success ? data.output.hash : undefined };
}
