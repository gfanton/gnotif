// The demo page's opt-in decisions, apart from the DOM.
import type { GnotifError, Trigger } from "gnotif";

import { addressProblem, isAddress } from "./address.ts";
import type { Settings } from "./settings.ts";

const addressMessages = {
  empty: "Enter the address you will send from, or connect Adena.",
  format: "This isn't a gno.land address. It starts with g1, followed by 38 lowercase letters and digits.",
  checksum: "This address has a typo: its checksum doesn't match. Check it, or paste it again.",
} as const satisfies Record<NonNullable<ReturnType<typeof addressProblem>>, string>;

/** addressMessage is what step 1 shows under the address field, "" for an address. */
export function addressMessage(address: string): string {
  const problem = addressProblem(address);
  return problem === undefined ? "" : addressMessages[problem];
}

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
  const problem = addressMessage(address);
  if (problem !== "") {
    throw new Error(problem);
  }
  return [{ trigger: trigger.id, value: address }];
}

/**
 * sendHint is the hint under step 2's send controls: what sending waits for,
 * "" once it can send.
 * @param ready whether a delivery channel is opted into address
 * @param trigger null until it loads
 */
export function sendHint(address: string, ready: boolean, trigger: Trigger | null): string {
  if (trigger === null) {
    return "Sending waits for the echo trigger to load.";
  }
  if (!isAddress(address)) {
    return address === "" ? "Set your address in step 1 to continue." : "Fix the address in step 1 to continue.";
  }
  return ready ? "" : "Turn on notifications or live updates for this address in step 1 to continue.";
}

/**
 * pushSupported reports whether this browser can show the page's
 * notifications: it needs a service worker, the Push API and notifications.
 * iPhone and iPad offer the Push API only to sites added to the Home Screen.
 * @param scope globalThis
 */
export function pushSupported(scope: { navigator: object; PushManager?: unknown; Notification?: unknown }): boolean {
  return "serviceWorker" in scope.navigator && "PushManager" in scope && "Notification" in scope;
}

/** pushHelp is step 1's line when the browser cannot show notifications, "" when it can. */
export function pushHelp(supported: boolean): string {
  return supported ? "" : "Use live updates below to see your messages here.";
}

/**
 * turnOnIsNext reports whether "Turn on notifications" is step 1's next
 * action, the one amber control: not while notifications or live updates
 * receive already, nor while the browser cannot or will not show them.
 * @param state blocked: the site's permission is denied
 */
export function turnOnIsNext({
  on,
  live,
  supported,
  blocked,
}: {
  on: boolean;
  live: boolean;
  supported: boolean;
  blocked: boolean;
}): boolean {
  return !on && !live && supported && !blocked;
}

/**
 * pushStopped reports whether step 1 should say this browser stopped
 * receiving notifications: they were on here before, are off now, and turning
 * them on again can work.
 * @param state wasOn: the saved settings had them on; blocked: the site's
 *   permission is denied
 */
export function pushStopped({
  wasOn,
  on,
  supported,
  blocked,
}: {
  wasOn: boolean;
  on: boolean;
  supported: boolean;
  blocked: boolean;
}): boolean {
  return wasOn && !on && supported && !blocked;
}

/**
 * pushNoteAtLoad is step 1's line under the notification buttons once the
 * page has loaded: why Turn on cannot run yet, or that notifications stopped,
 * "" when there is nothing to say.
 * @param state on: this browser receives them; stopped: what pushStopped reports
 */
export function pushNoteAtLoad({
  on,
  supported,
  triggerLoaded,
  stopped,
}: {
  on: boolean;
  supported: boolean;
  triggerLoaded: boolean;
  stopped: boolean;
}): string {
  if (on) {
    return "";
  }
  if (!supported) {
    return pushHelp(false);
  }
  if (!triggerLoaded) {
    return "Turning on waits for the echo trigger to load.";
  }
  return stopped ? "This browser stopped receiving notifications. Turn them on again to resume." : "";
}

/**
 * notificationState is step 1's line about notifications in this browser.
 * @param state on: this browser receives them; blocked: the site's permission
 *   is denied; address: the address they are on for, "" when unknown
 */
export function notificationState({
  supported,
  on,
  blocked,
  address,
}: {
  supported: boolean;
  on: boolean;
  blocked: boolean;
  address: string;
}): string {
  if (on) {
    return address === "" ? "Notifications on" : `Notifications on for ${address}`;
  }
  if (!supported) {
    return "This browser can't show notifications from this page";
  }
  return blocked ? "Notifications blocked in this browser's site settings" : "Notifications off";
}

/**
 * pushErrorText is step 1's message when turning notifications on fails. Where
 * notifications cannot work, it points to live updates, which need neither.
 * @param code the GnotifError's code, undefined for another error
 * @param permission Notification.permission after the attempt
 * @param message the error's own message, which other failures show
 */
export function pushErrorText(
  code: GnotifError["code"] | undefined,
  permission: string | undefined,
  message: string,
): string {
  if (code === "unsupported") {
    return "This browser can't show notifications from this page. Use live updates below to see your messages here.";
  }
  if (code === "denied") {
    return permission === "denied"
      ? "Notifications are blocked for this site. Allow them in the browser's site settings, or use live updates below."
      : "The browser did not get permission. Try again and allow notifications, or use live updates below.";
  }
  return message;
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
