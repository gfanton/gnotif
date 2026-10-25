// What step 2's send controls show. Adena sends only from the account it
// connected in step 1; an address typed by hand, or a browser without Adena,
// sends with gnokey.

/**
 * @param state adena: whether the extension was found yet; wallet: the
 *   connected Adena address, "" when none; address: step 1's field
 * @returns adenaSend: how Send with Adena shows; gnokeyLeads: whether Copy the
 *   gnokey command leads, with the gnokey details open
 */
export function sendArea({
  adena,
  wallet,
  address,
}: {
  adena: "pending" | "found" | "absent";
  wallet: string;
  address: string;
}): { adenaSend: "enabled" | "disabled" | "hidden"; gnokeyLeads: boolean } {
  if (adena !== "found") {
    return { adenaSend: "hidden", gnokeyLeads: true };
  }
  if (wallet !== "") {
    return { adenaSend: "enabled", gnokeyLeads: false };
  }
  return address === "" ? { adenaSend: "disabled", gnokeyLeads: false } : { adenaSend: "hidden", gnokeyLeads: true };
}
