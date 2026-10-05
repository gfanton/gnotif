const CHARSET = "qpzry9x8gf2tvdw0s3jn54khce6mua7l";
const GENERATOR = [0x3b6a57b2, 0x26508e6d, 0x1ea119fa, 0x3d4233dd, 0x2a1462b3];
const ADDRESS = /^g1[qpzry9x8gf2tvdw0s3jn54khce6mua7l]{38}$/;

// polymod is the BIP-173 checksum: https://github.com/bitcoin/bips/blob/master/bip-0173.mediawiki
/** @param {number[]} values */
function polymod(values) {
  let chk = 1;
  for (const v of values) {
    const top = chk >>> 25;
    chk = ((chk & 0x1ffffff) << 5) ^ v;
    GENERATOR.forEach((g, i) => {
      if ((top >>> i) & 1) {
        chk ^= g;
      }
    });
  }
  return chk;
}

/**
 * isAddress reports whether s is a well-formed gno address: bech32, prefix "g", 20 bytes.
 * @param {string} s
 * @returns {boolean}
 */
export function isAddress(s) {
  if (!ADDRESS.test(s)) {
    return false;
  }
  const hrp = [...s.slice(0, 1)].map((c) => c.charCodeAt(0));
  const data = [...s.slice(2)].map((c) => CHARSET.indexOf(c));
  const expand = [...hrp.map((c) => c >> 5), 0, ...hrp.map((c) => c & 31)];
  return polymod([...expand, ...data]) === 1;
}
