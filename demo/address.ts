const CHARSET = "qpzry9x8gf2tvdw0s3jn54khce6mua7l";
const GENERATOR = [0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3];
const ADDRESS = /^g1[qpzry9x8gf2tvdw0s3jn54khce6mua7l]{38}$/;

// polymod is the BIP-173 checksum: https://github.com/bitcoin/bips/blob/master/bip-0173.mediawiki
function polymod(values: number[]): number {
  let chk = 1;
  for (const v of values) {
    const top = chk >>> 25;
    chk = ((chk & 0x1ffffff) << 5) ^ v;
    GENERATOR.forEach((g, i) => {
      if (((top >>> i) & 1) === 1) {
        chk ^= g;
      }
    });
  }
  return chk;
}

/**
 * addressProblem names what keeps s from being a gno address: empty, not g1
 * followed by 38 bech32 characters, or a checksum that does not match. It is
 * undefined when s is an address.
 */
export function addressProblem(s: string): "empty" | "format" | "checksum" | undefined {
  if (s === "") {
    return "empty";
  }
  if (!ADDRESS.test(s)) {
    return "format";
  }
  const hrp = Array.from(s.slice(0, 1), (c) => c.charCodeAt(0));
  const data = Array.from(s.slice(2), (c) => CHARSET.indexOf(c));
  const expand = [...hrp.map((c) => c >> 5), 0, ...hrp.map((c) => c & 31)];
  return polymod([...expand, ...data]) === 1 ? undefined : "checksum";
}

/** isAddress reports whether s is a well-formed gno address: bech32, prefix "g", 20 bytes. */
export function isAddress(s: string): boolean {
  return addressProblem(s) === undefined;
}
