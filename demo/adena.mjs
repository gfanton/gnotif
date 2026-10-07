// The Adena sequence, against the injected window.adena. Every answer is
// { status, type, message, data }; the page branches on `type` only, since
// the numeric codes differ between the extension and its SDK.
import { callMessage, gas } from "./echo-tx.mjs";

const SITE = "gnotif demo";

/** A failure Adena reported, with its `type` and a message for the visitor. */
export class AdenaError extends Error {
  /**
   * @param {string} type Adena's response type, or WRONG_CHAIN
   * @param {string} message
   */
  constructor(type, message) {
    super(message);
    this.name = "AdenaError";
    this.type = type;
  }
}

/**
 * @param {{ name: string }} chain
 * @param {string} detail Adena's own message, as ": <message>", or ""
 */
const messages = (chain, detail) => ({
  TRANSACTION_REJECTED: "You rejected the transaction in Adena.",
  WALLET_LOCKED: "Adena is locked. Unlock it, then try again.",
  NETWORK_TIMEOUT: `Adena could not reach ${chain.name}. Try again in a moment.`,
  TRANSACTION_FAILED: `The transaction failed on the chain${detail}.`,
  ACCOUNT_MISMATCH: "Adena's active account changed. Connect again.",
  UNADDED_NETWORK: `Adena does not know the ${chain.name} network.`,
});

/**
 * check returns the answer when Adena reports success, or one of the
 * accepted failure types, and throws an AdenaError otherwise.
 * @param {{ status: string, type: string, message?: string }} answer
 * @param {{ name: string }} chain
 * @param {string[]} accepted failure types that count as success here
 */
function check(answer, chain, accepted = []) {
  if (answer.status === "success" || accepted.includes(answer.type)) {
    return answer;
  }
  const detail = answer.message ? `: ${answer.message}` : "";
  const known = messages(chain, detail)[answer.type];
  throw new AdenaError(answer.type, known ?? `Adena answered ${answer.type}${detail}.`);
}

/**
 * connect establishes the site with Adena, moves it to chain, and returns the
 * active account's address. It refuses an account on another chain.
 * @param {object} adena window.adena
 * @param {{ id: string, name: string, rpc: string }} chain
 * @returns {Promise<string>}
 */
export async function connect(adena, chain) {
  check(await adena.AddEstablish(SITE), chain, ["ALREADY_CONNECTED"]);
  const network = check(await adena.GetNetwork(), chain);
  if (network.data.chainId !== chain.id) {
    let switched = await adena.SwitchNetwork(chain.id);
    if (switched.type === "UNADDED_NETWORK") {
      check(await adena.AddNetwork({ chainId: chain.id, chainName: chain.name, rpcUrl: chain.rpc }), chain);
      switched = await adena.SwitchNetwork(chain.id);
    }
    check(switched, chain, ["REDUNDANT_CHANGE_REQUEST"]);
  }
  const account = check(await adena.GetAccount(), chain);
  if (account.data.chainId !== chain.id) {
    throw new AdenaError("WRONG_CHAIN", `Adena's account is on ${account.data.chainId}, not ${chain.id}. Switch Adena to ${chain.name}, then try again.`);
  }
  return account.data.address;
}

/**
 * sendEcho connects, then has Adena sign and broadcast Echo(msg) from the
 * active account.
 * @param {object} adena window.adena
 * @param {{ id: string, name: string, rpc: string }} chain
 * @param {string} pkgPath the echo realm
 * @param {string} msg
 * @returns {Promise<{ caller: string, hash: string | undefined }>}
 */
export async function sendEcho(adena, chain, pkgPath, msg) {
  const caller = await connect(adena, chain);
  const sent = await adena.DoContract({
    messages: [callMessage(caller, pkgPath, msg)],
    gasFee: gas.fee,
    gasWanted: gas.wanted,
    memo: "",
  });
  if (sent.type !== "TRANSACTION_SUCCESS") {
    check({ ...sent, status: "failure" }, chain);
  }
  return { caller, hash: sent.data?.hash };
}
