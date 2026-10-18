/** Gas for one Echo call. Adena re-estimates in its popup, so these are hints there. */
export const gas = { wanted: 10000000, fee: 20000 };

/** shellQuote single-quotes s for a POSIX shell, so any text pastes safely. */
export function shellQuote(s: string): string {
  return `'${s.replaceAll("'", "'\\''")}'`;
}

/**
 * gnokeyCommand is the gnokey call that sends msg to Echo on chain, ending
 * with a placeholder for the visitor's key name.
 */
export function gnokeyCommand(chain: { id: string; rpc: string }, pkgPath: string, msg: string): string {
  return (
    `gnokey maketx call -pkgpath ${pkgPath} -func Echo -args ${shellQuote(msg)}` +
    ` -gas-wanted ${gas.wanted} -gas-fee ${gas.fee}ugnot` +
    ` -chainid ${chain.id} -remote ${chain.rpc} <your key>`
  );
}

/** callMessage is the /vm.m_call message that Adena signs for Echo. */
export function callMessage(
  caller: string,
  pkgPath: string,
  msg: string,
): {
  type: "/vm.m_call";
  value: { caller: string; send: string; max_deposit: string; pkg_path: string; func: string; args: string[] };
} {
  return {
    type: "/vm.m_call",
    value: { caller, send: "", max_deposit: "", pkg_path: pkgPath, func: "Echo", args: [msg] },
  };
}
