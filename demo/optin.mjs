// The demo page's opt-in decisions, apart from the DOM.
import { isAddress } from "./address.mjs";

/**
 * optinsFor is the opt-in list that sends trigger's notifications to address.
 * It throws an error for the visitor when the trigger is missing or the address
 * is empty or malformed.
 * @param {import("./gnotif.js").Trigger | null} trigger null until it loads
 * @param {string} address
 * @returns {{ trigger: string, value: string }[]}
 */
export function optinsFor(trigger, address) {
  if (trigger === null) {
    throw new Error("The echo trigger has not loaded. Reload the page, then try again.");
  }
  if (address === "") {
    throw new Error("Enter your address, or connect Adena.");
  }
  if (!isAddress(address)) {
    throw new Error(`"${address}" isn't a valid gno address. Expected g1 followed by 38 letters and digits.`);
  }
  return [{ trigger: trigger.id, value: address }];
}

/**
 * canEnable reports whether "Turn on notifications" can run.
 * @param {boolean} busy whether another action is in flight
 * @param {import("./gnotif.js").Trigger | null} trigger
 * @returns {boolean}
 */
export function canEnable(busy, trigger) {
  return !busy && trigger !== null;
}

/**
 * listenerMismatch returns the address this browser is opted in with when a
 * notification for caller misses it, or undefined when it reaches this browser
 * or the browser is not opted in.
 * @param {string} caller the address that sends the Echo call
 * @param {boolean} on whether this browser receives notifications
 * @param {{ address: string } | null} saved the opt-in this browser last set
 * @returns {string | undefined}
 */
export function listenerMismatch(caller, on, saved) {
  if (!on || saved === null || saved.address === caller) {
    return undefined;
  }
  return saved.address;
}
