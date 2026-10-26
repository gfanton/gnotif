// What step 1's address area shows. The address is typed, or comes from a
// connected Adena wallet, and it is locked while notifications are on, since
// they go to that address. Adena can still connect then, on that address only.

import { shortAddress } from "./adena.ts";

/**
 * @param state on: notifications are on; adena: whether the extension was
 *   found yet; wallet: the connected Adena address, "" when none
 */
export function addressArea({
  on,
  adena,
  wallet,
}: {
  on: boolean;
  adena: "pending" | "found" | "absent";
  wallet: string;
}): {
  input: "edit" | "readonly" | "hidden";
  wallet: boolean;
  disconnect: boolean;
  connect: boolean;
  getAdena: boolean;
  locked: boolean;
} {
  const connected = wallet !== "";
  return {
    input: connected ? "hidden" : on ? "readonly" : "edit",
    wallet: connected,
    disconnect: connected && !on,
    connect: !connected && adena === "found",
    getAdena: !on && !connected && adena === "absent",
    locked: on,
  };
}

/**
 * lockedRefusal says why Adena's account cannot connect while notifications
 * are on for another address, or returns "" when it can.
 * @param address step 1's address
 * @param account Adena's active account
 */
export function lockedRefusal(on: boolean, address: string, account: string): string {
  if (!on || account === address) {
    return "";
  }
  return `Adena's active account is ${shortAddress(account)}, but notifications are on for ${shortAddress(address)}. Switch Adena to that account, or turn notifications off first.`;
}
