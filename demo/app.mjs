import { Gnotif, GnotifError } from "./gnotif.js";
import { connect, sendEcho } from "./adena.mjs";
import { gnokeyCommand } from "./echo-tx.mjs";
import { chain, echo, gnoweb, server } from "./config.js";
import { canEnable, listenerMismatch, optinsFor } from "./optin.mjs";

const gnotif = new Gnotif({ server, serviceWorker: "/sw.js" });
const el = (id) => document.getElementById(id);
// The server cannot read opt-ins back, so the page keeps the last one it set.
const STORAGE_KEY = "gnotif-demo";
const realmURL = gnoweb.replace(/\/$/, "") + echo.replace(/^gno\.land/, "");
const adena = globalThis.adena;
let trigger = null;
let on = false;
let pending = Promise.resolve();

/** @returns {{ address: string, optins: { trigger: string, value: string }[] } | null} */
function stored() {
  return JSON.parse(localStorage.getItem(STORAGE_KEY) ?? "null");
}

function save(optins) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify({ address: address(), optins }));
}

function address() {
  return el("address").value.trim();
}

function message() {
  return el("msg").value.trim() || el("msg").placeholder;
}

function short(a) {
  return `${a.slice(0, 6)}…${a.slice(-4)}`;
}

function say(id, text, isError = false) {
  el(id).textContent = text;
  el(id).classList.toggle("error", isError);
}

function errorText(err) {
  if (err instanceof GnotifError && err.code === "denied") {
    return Notification.permission === "denied"
      ? "Notifications are blocked for this site. Allow them in the browser's site settings, then try again."
      : "The browser did not get permission. Try again and allow notifications.";
  }
  return err.message;
}

function optin() {
  return optinsFor(trigger, address());
}

async function setOptin() {
  const optins = optin();
  await gnotif.setOptins(optins);
  save(optins);
}

// One amber button at a time: the next action in the flow.
function setPrimary() {
  el("enable").classList.toggle("primary", !on);
  el("send").classList.toggle("primary", on && Boolean(adena));
  el("copy").classList.toggle("primary", on && !adena);
}

async function refresh() {
  on = await gnotif.enabled();
  const saved = stored();
  const blocked = "Notification" in globalThis && Notification.permission === "denied";
  el("state").classList.toggle("on", on);
  el("state-text").textContent = on && saved
    ? `On for ${saved.address}`
    : on ? "On" : blocked ? "Blocked in the browser's site settings" : "Off";
  el("enable").textContent = saved && !on ? "Turn notifications on again" : "Turn on notifications";
  el("enable").hidden = on;
  el("disable").hidden = !on;
  setPrimary();
}

// Opt-in changes run one at a time, so the last change is the one the server keeps.
function sync() {
  pending = pending.then(async () => {
    try {
      await setOptin();
      say("message", `Notifications now go to ${short(address())}.`);
    } catch (err) {
      say("message", errorText(err), true);
    }
    await refresh();
  }).catch((err) => say("message", err.message, true));
}

function busy(state) {
  for (const id of ["disable", "connect", "send"]) {
    el(id).disabled = state;
  }
  el("enable").disabled = !canEnable(state, trigger);
}

function noteMismatch(caller) {
  const listening = listenerMismatch(caller, on, stored());
  say("mismatch", listening === undefined
    ? ""
    : `Adena sends from ${short(caller)}, but this browser listens for ${short(listening)}. The notification goes to ${short(caller)}.`);
}

function renderMessage() {
  const msg = message();
  el("toast-body").textContent = trigger ? trigger.body.replaceAll("{msg}", msg) : msg;
  el("command").textContent = gnokeyCommand(chain, echo, msg);
}

el("enable").addEventListener("click", async () => {
  try {
    optin();
  } catch (err) {
    say("message", err.message, true);
    el("address").focus();
    return;
  }
  busy(true);
  say("message", "Turning on notifications…");
  try {
    await gnotif.enable();
    await setOptin();
    say("message", "Notifications on. Send yourself a message in step 2.");
  } catch (err) {
    say("message", errorText(err), true);
  }
  busy(false);
  await refresh();
});

el("disable").addEventListener("click", async () => {
  busy(true);
  try {
    await gnotif.disable();
    localStorage.removeItem(STORAGE_KEY);
    say("message", "Notifications off.");
  } catch (err) {
    say("message", errorText(err), true);
  }
  busy(false);
  await refresh();
});

el("connect").addEventListener("click", async () => {
  busy(true);
  say("message", "Waiting for Adena…");
  try {
    el("address").value = await connect(adena, chain);
    say("message", `Adena connected as ${short(address())}.`);
    say("mismatch", "");
    if (on) {
      sync();
    }
  } catch (err) {
    say("message", err.message, true);
  }
  busy(false);
});

el("send").addEventListener("click", async () => {
  if (!el("msg").value.trim()) {
    say("sent", "Write a message first.", true);
    el("msg").focus();
    return;
  }
  busy(true);
  say("mismatch", "");
  say("sent", "Waiting for Adena…");
  try {
    const { caller, hash } = await sendEcho(adena, chain, echo, message());
    noteMismatch(caller);
    say("sent", on
      ? `Sent. The notification arrives in a few seconds.${hash ? ` Transaction ${hash}.` : ""}`
      : "Sent, but notifications are off in this browser. Turn them on in step 1 to get the next one.");
  } catch (err) {
    say("sent", err.message, true);
  }
  busy(false);
});

el("copy").addEventListener("click", async () => {
  try {
    await navigator.clipboard.writeText(el("command").textContent);
    el("copy").textContent = "Copied";
    setTimeout(() => { el("copy").textContent = "Copy"; }, 1500);
  } catch {
    say("sent", "Copying failed. Select the command and copy it yourself.", true);
  }
});

el("address").addEventListener("change", () => {
  say("mismatch", "");
  if (on) {
    sync();
  }
});

el("msg").addEventListener("input", renderMessage);

el("chain-name").textContent = chain.name;
el("chain-id").textContent = chain.id;
el("realm-link").href = realmURL;
el("realm-link-foot").href = realmURL;
el("server").textContent = server;
el("toast-host").textContent = location.host;
el("connect").hidden = !adena;
el("send").hidden = !adena;
renderMessage();

const saved = stored();
if (saved) {
  el("address").value = saved.address;
}
try {
  [trigger = null] = await gnotif.triggers(echo);
  if (trigger === null) {
    say("message", `${server} lists no trigger for ${echo} yet. Deploy the echo realm first.`, true);
  } else {
    el("toast-title").textContent = trigger.title;
    renderMessage();
  }
} catch (err) {
  say("message", `Cannot load the echo trigger from ${server}: ${err.message}`, true);
}
el("enable").disabled = !canEnable(false, trigger);
try {
  await refresh();
  if (!on && saved) {
    say("message", "This browser stopped receiving notifications. Turn them on again to resume.");
  }
} catch (err) {
  say("message", err.message, true);
}
