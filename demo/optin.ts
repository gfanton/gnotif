// The demo page's opt-in decisions, apart from the DOM.
import type { Trigger } from "gnotif";

import { isAddress } from "./address.ts";
import type { Settings } from "./settings.ts";

/**
 * optinsFor is the opt-in list that sends trigger's notifications to address.
 * It throws an error for the visitor when the trigger is missing or the address
 * is empty or malformed.
 * @param trigger null until it loads
 */
export function optinsFor(trigger: Trigger | null, address: string): { trigger: string; value: string }[] {
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
 * @param busy whether another action is in flight
 */
export function canEnable(busy: boolean, trigger: Trigger | null): boolean {
  return !busy && trigger !== null;
}

/**
 * optinsAtLoad reports whether the page sets its opt-ins as it loads: when
 * push is on, the trigger has loaded, and live updates do not resume, since
 * resuming them sets the opt-ins. Without them the client drops the pushes
 * the service worker passes to the page.
 * @param on whether this browser receives notifications
 * @param resuming whether live updates resume at load
 * @param trigger null until it loads
 */
export function optinsAtLoad(on: boolean, resuming: boolean, trigger: Trigger | null): boolean {
  return on && !resuming && trigger !== null;
}

/**
 * listenerMismatch returns the address this browser is opted in with when a
 * notification for caller misses it, or undefined when it reaches this browser
 * or the browser is not opted in.
 * @param caller the address that sends the Echo call
 * @param on whether this browser receives notifications
 * @param saved the opt-in this browser last set
 */
export function listenerMismatch(
  caller: string,
  on: boolean,
  saved: Pick<Settings, "address"> | null,
): string | undefined {
  if (!on || saved === null || saved.address === caller) {
    return undefined;
  }
  return saved.address;
}

/** Sending needs an active delivery channel opted into this address and trigger. */
export function canSend(
  trigger: Trigger | null,
  address: string,
  receiving: boolean,
  optins: Settings["optins"],
): boolean {
  return (
    receiving &&
    trigger !== null &&
    isAddress(address) &&
    optins.some((optin) => optin.trigger === trigger.id && optin.value === address)
  );
}
