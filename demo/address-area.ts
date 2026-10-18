// What step 1's address area shows. The address is typed, or comes from a
// connected Adena wallet, and it is locked while notifications are on, since
// they go to that address.

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
  const free = !on && !connected;
  return {
    input: connected ? "hidden" : on ? "readonly" : "edit",
    wallet: connected,
    disconnect: connected && !on,
    connect: free && adena === "found",
    getAdena: free && adena === "absent",
    locked: on,
  };
}
