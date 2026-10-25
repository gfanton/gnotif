import { test } from "node:test";
import assert from "node:assert/strict";
import {
  addressMessage,
  canSend,
  canEnable,
  notificationState,
  optinsAtLoad,
  optinsFor,
  pushErrorText,
  pushHelp,
  pushNoteAtLoad,
  pushStopped,
  pushSupported,
  sendHint,
  turnOnIsNext,
} from "./optin.ts";

const trigger = {
  id: "7",
  target: "gno.land/r/dev/echo/v1",
  event: "Echo",
  filter: "",
  param: "to",
  title: "Echo",
  body: "{msg}",
  link: "/",
  declarer: "g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5",
  verified: true,
};
const address = "g1zrjyzatudmadgqu39d5wfamhdvf0adwlxkz25d";
const other = "g13rqvks880v0hmlfepcj0kkudq0js38d3pc5k6a";

test("optinsFor opts the address into the trigger", () => {
  assert.deepEqual(optinsFor(trigger, address), [{ trigger: "7", value: address }]);
});

test("optinsFor refuses before the trigger has loaded", () => {
  assert.throws(() => optinsFor(null, address), {
    message: "The echo trigger has not loaded. Reload the page, then try again.",
  });
});

test("optinsFor refuses an empty or malformed address with step 1's message", () => {
  assert.throws(() => optinsFor(trigger, ""), { message: addressMessage("") });
  assert.throws(() => optinsFor(trigger, "g1nope"), { message: addressMessage("g1nope") });
});

test("addressMessage tells an empty field, a wrong format and a typo apart", () => {
  assert.equal(addressMessage(""), "Enter the address you will send from, or connect Adena.");
  assert.equal(
    addressMessage("g1abc"),
    "This isn't a gno.land address. It starts with g1, followed by 38 lowercase letters and digits.",
  );
  assert.equal(
    addressMessage(`${address.slice(0, -1)}q`),
    "This address has a typo: its checksum doesn't match. Check it, or paste it again.",
  );
  assert.equal(addressMessage(address), "");
});

test("sendHint names what step 2 waits for, and nothing once it can send", () => {
  assert.equal(sendHint("", false, trigger), "Set your address in step 1 to continue.");
  assert.equal(sendHint("g1abc", false, trigger), "Fix the address in step 1 to continue.");
  assert.equal(
    sendHint(address, false, trigger),
    "Turn on notifications or live updates for this address in step 1 to continue.",
  );
  assert.equal(sendHint(address, true, trigger), "");
});

test("sendHint waits for the echo trigger before anything step 1 could fix", () => {
  assert.equal(sendHint("", false, null), "Sending waits for the echo trigger to load.");
  assert.equal(sendHint(address, false, null), "Sending waits for the echo trigger to load.");
});

test("without Web Push, step 1 points to live updates", () => {
  assert.equal(pushHelp(false), "Use live updates below to see your messages here.");
  assert.equal(pushHelp(true), "");
});

test("pushSupported needs a service worker, the Push API and notifications", () => {
  const full = { navigator: { serviceWorker: {} }, PushManager: class {}, Notification: class {} };
  assert.equal(pushSupported(full), true);
  assert.equal(pushSupported({ ...full, navigator: {} }), false, "no service worker");
  assert.equal(
    pushSupported({ navigator: full.navigator, Notification: full.Notification }),
    false,
    "no PushManager, as on iPhone Safari",
  );
  assert.equal(pushSupported({ navigator: full.navigator, PushManager: full.PushManager }), false, "no Notification");
});

test("notificationState names the channel and its state", () => {
  const off = { supported: true, on: false, blocked: false, address: "" };
  assert.equal(notificationState(off), "Notifications off");
  assert.equal(notificationState({ ...off, on: true, address }), `Notifications on for ${address}`);
  assert.equal(notificationState({ ...off, on: true }), "Notifications on");
  assert.equal(notificationState({ ...off, blocked: true }), "Notifications blocked in this browser's site settings");
  assert.equal(notificationState({ ...off, supported: false }), "This browser can't show notifications from this page");
});

test("a failed Turn on points to live updates when notifications cannot work", () => {
  assert.equal(
    pushErrorText("unsupported", undefined, "This browser has no Push API."),
    "This browser can't show notifications from this page. Use live updates below to see your messages here.",
  );
  assert.equal(
    pushErrorText("denied", "denied", "Notifications are not allowed for this site."),
    "Notifications are blocked for this site. Allow them in the browser's site settings, or use live updates below.",
  );
  assert.equal(
    pushErrorText("denied", "default", "Notifications are not allowed for this site."),
    "The browser did not get permission. Try again and allow notifications, or use live updates below.",
  );
  assert.equal(
    pushErrorText("server", "granted", "gnotif server sent a malformed VAPID key"),
    "gnotif server sent a malformed VAPID key",
  );
  assert.equal(pushErrorText(undefined, "granted", "Failed to fetch"), "Failed to fetch");
});

test("canEnable needs an idle page and a loaded trigger", () => {
  assert.equal(canEnable(false, trigger), true);
  assert.equal(canEnable(true, trigger), false, "busy");
  assert.equal(canEnable(false, null), false, "no trigger");
});

test("optinsAtLoad sets the opt-ins as the page loads once push is on and the trigger has loaded", () => {
  assert.equal(optinsAtLoad(true, false, trigger), true);
  assert.equal(optinsAtLoad(false, false, trigger), false, "push off");
  assert.equal(optinsAtLoad(true, false, null), false, "no trigger");
});

test("optinsAtLoad leaves the opt-ins to live updates that resume at load", () => {
  assert.equal(optinsAtLoad(true, true, trigger), false);
});

test("sending requires a valid address and its active opt-in, through push or live", () => {
  const optins = [{ trigger: trigger.id, value: address }];
  assert.equal(canSend(trigger, address, true, optins), true);
  assert.equal(canSend(trigger, address, false, optins), false);
  assert.equal(canSend(null, address, true, optins), false);
  assert.equal(canSend(trigger, "", true, optins), false);
  assert.equal(canSend(trigger, "g1nope", true, optins), false);
  assert.equal(canSend(trigger, other, true, optins), false);
  assert.equal(canSend(trigger, address, true, []), false);
  assert.equal(canSend(trigger, address, true, [{ trigger: "another", value: address }]), false);
});

test("Turn on notifications is the next action only while it can work and nothing receives yet", () => {
  const off = { on: false, live: false, supported: true, blocked: false };
  assert.equal(turnOnIsNext(off), true);
  assert.equal(turnOnIsNext({ ...off, on: true }), false, "already on");
  assert.equal(turnOnIsNext({ ...off, live: true }), false, "live updates already receive; step 2 is next");
  assert.equal(turnOnIsNext({ ...off, supported: false }), false, "no Push API");
  assert.equal(turnOnIsNext({ ...off, blocked: true }), false, "blocked until the site settings change");
});

test("as the page loads, step 1 says why Turn on cannot run yet, or that notifications stopped", () => {
  const ready = { on: false, supported: true, triggerLoaded: true, stopped: false };
  assert.equal(pushNoteAtLoad(ready), "");
  assert.equal(pushNoteAtLoad({ ...ready, on: true, triggerLoaded: false }), "", "on already: nothing to turn on");
  assert.equal(
    pushNoteAtLoad({ ...ready, stopped: true }),
    "This browser stopped receiving notifications. Turn them on again to resume.",
  );
  assert.equal(pushNoteAtLoad({ ...ready, triggerLoaded: false }), "Turning on waits for the echo trigger to load.");
  assert.equal(
    pushNoteAtLoad({ ...ready, triggerLoaded: false, stopped: true }),
    "Turning on waits for the echo trigger to load.",
    "Turn on cannot run before the trigger loads",
  );
  assert.equal(
    pushNoteAtLoad({ ...ready, supported: false, triggerLoaded: false }),
    "Use live updates below to see your messages here.",
    "without the Push API, the trigger changes nothing",
  );
});

test("notifications read as stopped only when this browser could turn them on again", () => {
  const stopped = { wasOn: true, on: false, supported: true, blocked: false };
  assert.equal(pushStopped(stopped), true);
  assert.equal(pushStopped({ ...stopped, wasOn: false }), false, "never on in this browser");
  assert.equal(pushStopped({ ...stopped, on: true }), false, "still on");
  assert.equal(pushStopped({ ...stopped, supported: false }), false, "no Push API: live updates are the way");
  assert.equal(pushStopped({ ...stopped, blocked: true }), false, "blocked until the site settings change");
});
