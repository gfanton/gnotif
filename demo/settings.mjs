// What the demo remembers in this browser. The server cannot read opt-ins
// back, so the page keeps the last ones it set.

const KEY = "gnotif-demo";

/**
 * @typedef {object} Settings
 * @property {string} address
 * @property {{ trigger: string, value: string }[]} optins
 * @property {boolean} live whether the visitor turned live updates on
 * @property {boolean} push whether the visitor turned push notifications on
 */

/**
 * Reads the stored settings. A record without `push` reads as push on.
 * @param {Pick<Storage, "getItem">} storage
 * @returns {Settings | null} null when nothing usable is stored
 */
export function load(storage) {
  let v;
  try {
    v = JSON.parse(storage.getItem(KEY) ?? "null");
  } catch {
    return null;
  }
  if (typeof v !== "object" || v === null) {
    return null;
  }
  return {
    address: typeof v.address === "string" ? v.address : "",
    optins: Array.isArray(v.optins) ? v.optins : [],
    live: v.live === true,
    push: v.push !== false,
  };
}

/**
 * @param {Pick<Storage, "setItem">} storage
 * @param {Settings} settings
 */
export function save(storage, settings) {
  storage.setItem(KEY, JSON.stringify(settings));
}
