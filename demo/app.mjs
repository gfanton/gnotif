import { Gnotif, GnotifError } from "./gnotif.js";
import { connect, findAdena, sendEcho, shortAddress as short, walletLabel } from "./adena.mjs";
import { gnokeyCommand } from "./echo-tx.mjs";
import { chain, echo, gnoweb, gnotif as options } from "./config.js";
import { canEnable, listenerMismatch, optinsFor } from "./optin.mjs";
import { addressArea } from "./address-area.mjs";
import { load, save } from "./settings.mjs";
import { WAIT_MS, answeredSend, blockSummary, clockText, deliveredBody, details } from "./live-log.mjs";

const gnotif = new Gnotif({ ...options, serviceWorker: "/sw.js", icon: "/icon.png" });
const el = (id) => document.getElementById(id);
const realmURL = gnoweb.replace(/\/$/, "") + echo.replace(/^gno\.land/, "");
/** @type {import("./adena.mjs").Adena | undefined} */
let adena;
/** @type {"pending" | "found" | "absent"} */
let adenaState = "pending";
/** The connected Adena address, "" when none. */
let wallet = "";
let trigger = null;
let on = false; // push is on
let live = false; // live updates are on

function stored() {
  return load(localStorage);
}

// remember stores the address as typed, with changes over what is stored.
function remember(changes) {
  const s = stored() ?? { address: "", optins: [], live: false, push: false };
  save(localStorage, { ...s, address: address(), ...changes });
}

function address() {
  return el("address").value.trim();
}

function message() {
  return el("msg").value.trim() || el("msg").placeholder;
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
  remember({ optins });
}

function setLiveState(text, connected = false) {
  el("live-state").classList.toggle("on", connected);
  el("live-state-text").textContent = text;
}

async function startLive() {
  setLiveState("Connecting…");
  await setOptin();
  await gnotif.connect();
  live = true;
  setLiveState("Live updates on", true);
  el("arrivals").hidden = false;
}

function stopLive() {
  gnotif.disconnect();
  live = false;
  el("live").checked = false;
  setLiveState("Live updates off");
  el("arrivals").hidden = true;
  for (const sent of waiting) {
    clearInterval(sent.clock);
  }
  waiting.length = 0;
  el("arrivals-list").replaceChildren();
}

const ARRIVALS_KEPT = 5;
/** Sends waiting for their live arrival, oldest first. */
const waiting = [];

function stamp(time, at) {
  time.dateTime = at.toISOString();
  time.textContent = at.toLocaleTimeString();
}

// logRow puts a row on top of the log and drops rows past ARRIVALS_KEPT; a
// dropped waiting row stops waiting.
function logRow(title, body, at) {
  const time = document.createElement("time");
  stamp(time, at);
  const head = document.createElement("b");
  head.textContent = title;
  const text = document.createElement("span");
  text.textContent = body;
  const note = document.createElement("small");
  note.className = "note";
  const item = document.createElement("li");
  item.className = "arrival";
  item.append(time, head, text, note);
  const list = el("arrivals-list");
  list.prepend(item);
  while (list.children.length > ARRIVALS_KEPT) {
    const gone = list.lastElementChild;
    gone.remove();
    const i = waiting.findIndex((sent) => sent.row === gone);
    if (i !== -1) {
      clearInterval(waiting[i].clock);
      waiting.splice(i, 1);
    }
  }
  return item;
}

function addSent(body) {
  const at = new Date();
  const row = logRow(el("toast-title").textContent, body, at);
  row.classList.add("pending");
  const dot = document.createElement("i");
  dot.className = "indicator";
  const clock = document.createTextNode(clockText(0));
  // Hidden from screen readers: the log would read the clock out every second.
  const ticking = document.createElement("span");
  ticking.setAttribute("aria-hidden", "true");
  ticking.append(dot, clock);
  row.querySelector("time").replaceChildren(ticking);
  row.querySelector(".note").textContent = "Waiting for it to arrive";
  const tick = () => { clock.data = clockText(Date.now() - at.getTime()); };
  const sent = { row, body, at: at.getTime(), clock: setInterval(tick, 1000) };
  waiting.push(sent);
  setTimeout(() => {
    if (waiting.includes(sent)) {
      sent.late = true;
      clearInterval(sent.clock);
      clock.data = clockText(WAIT_MS);
      row.classList.replace("pending", "late");
      row.querySelector(".note").textContent =
        `It has not arrived after ${WAIT_MS / 1000} s. Check that Adena sends from the address in step 1.`;
    }
  }, WAIT_MS);
}

function addArrival(n) {
  const at = new Date();
  const i = answeredSend(waiting, n.body);
  let row;
  let wait;
  if (i === -1) {
    row = logRow(n.title, n.body, at);
  } else {
    const [sent] = waiting.splice(i, 1);
    clearInterval(sent.clock);
    row = sent.row;
    row.classList.remove("pending", "late");
    stamp(row.querySelector("time"), at);
    wait = at.getTime() - sent.at;
  }
  row.querySelector(".note").replaceWith(arrivalDetails(n, wait));
}

function arrivalDetails(n, wait) {
  const summary = document.createElement("summary");
  summary.textContent = blockSummary(n.height, wait);
  const list = document.createElement("dl");
  for (const [label, value] of details(n, (d) => d.toLocaleString())) {
    const dt = document.createElement("dt");
    dt.textContent = label;
    const dd = document.createElement("dd");
    dd.textContent = value;
    list.append(dt, dd);
  }
  const box = document.createElement("details");
  box.className = "meta";
  box.append(summary, list);
  return box;
}

function pointToLog() {
  const link = document.createElement("a");
  link.href = "#arrivals";
  link.textContent = "Watch for it in the Arrived live log ↑";
  el("sent").classList.remove("error");
  el("sent").replaceChildren("Sent. ", link);
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
  el("enable").textContent = saved?.push && !on ? "Turn notifications on again" : "Turn on notifications";
  el("enable").hidden = on;
  el("disable").hidden = !on;
  renderAddress();
  setPrimary();
}

function renderAddress() {
  const area = addressArea({ on, adena: adenaState, wallet });
  el("address-row").hidden = area.input === "hidden";
  el("address").readOnly = area.input === "readonly";
  el("connect").hidden = !area.connect;
  el("wallet").hidden = !area.wallet;
  el("wallet-address").textContent = wallet;
  el("disconnect").hidden = !area.disconnect;
  el("get-adena").hidden = !area.getAdena;
  el("address-hint").hidden = area.locked;
  el("locked-hint").hidden = !area.locked;
}

function busy(state) {
  for (const id of ["disable", "connect", "disconnect", "send"]) {
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

function notificationBody(msg) {
  return trigger !== null ? trigger.body.replaceAll("{msg}", msg) : msg;
}

function renderMessage() {
  const msg = message();
  el("toast-body").textContent = notificationBody(msg);
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
    remember({ push: true });
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
    remember({ push: false });
    say("message", "Notifications off.");
  } catch (err) {
    say("message", errorText(err), true);
  }
  busy(false);
  await refresh();
});

el("connect").addEventListener("click", async () => {
  const before = el("connect").textContent;
  busy(true);
  el("connect").textContent = walletLabel("pending");
  say("message", "Waiting for Adena…");
  try {
    wallet = await connect(adena, chain);
    el("address").value = wallet;
    say("message", `Adena connected as ${short(wallet)}.`);
    say("mismatch", "");
  } catch (err) {
    say("message", err.message, true);
  }
  el("connect").textContent = before;
  busy(false);
  renderAddress();
});

el("disconnect").addEventListener("click", () => {
  wallet = "";
  say("message", "Adena disconnected. Type an address, or connect again.");
  renderAddress();
});

el("send").addEventListener("click", async () => {
  if (!el("msg").value.trim()) {
    say("sent", "Write a message first.", true);
    el("msg").focus();
    return;
  }
  const before = el("send").textContent;
  busy(true);
  el("send").textContent = walletLabel("pending");
  say("mismatch", "");
  say("sent", "Waiting for Adena…");
  try {
    const msg = message();
    const { caller, hash } = await sendEcho(adena, chain, echo, msg);
    noteMismatch(caller);
    if (live) {
      addSent(deliveredBody(notificationBody(msg)));
      pointToLog();
    } else {
      say("sent", on
        ? `Sent. The notification arrives in a few seconds.${hash ? ` Transaction ${hash}.` : ""}`
        : "Sent, but notifications are off in this browser. Turn them on in step 1 to get the next one.");
    }
  } catch (err) {
    say("sent", err.message, true);
  }
  el("send").textContent = before;
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

el("msg").addEventListener("input", renderMessage);

el("live").addEventListener("change", async () => {
  if (!el("live").checked) {
    stopLive();
    remember({ live: false });
    say("message", "Live updates off.");
    return;
  }
  try {
    optin();
  } catch (err) {
    el("live").checked = false;
    say("message", err.message, true);
    el("address").focus();
    return;
  }
  try {
    await startLive();
    remember({ live: true });
    say("message", "Live updates on. Send yourself a message in step 2.");
  } catch (err) {
    stopLive();
    say("message", errorText(err), true);
  }
});

// While live updates are on, a new address replaces the opt-in at once.
el("address").addEventListener("change", async () => {
  if (!live) {
    return;
  }
  try {
    await setOptin();
  } catch (err) {
    say("message", errorText(err), true);
  }
});

gnotif.on("notification", addArrival);
gnotif.on("disconnect", () => setLiveState("Reconnecting…"));
gnotif.on("reconnect", () => setLiveState("Live updates on", true));
gnotif.on("error", (err) => say("message", err.message, true));

el("chain-name").textContent = chain.name;
el("chain-id").textContent = chain.id;
el("realm-link").href = realmURL;
el("realm-link-foot").href = realmURL;
el("server").textContent = gnotif.server;
el("toast-host").textContent = location.host;
el("code-realm").textContent = echo;
void findAdena(globalThis).then((found) => {
  adena = found;
  adenaState = found === undefined ? "absent" : "found";
  el("send-row").hidden = found === undefined;
  renderAddress();
  setPrimary();
});
renderMessage();

const saved = stored();
if (saved) {
  el("address").value = saved.address;
}
try {
  [trigger = null] = await gnotif.triggers(echo);
  if (trigger === null) {
    say("message", `${gnotif.server} lists no trigger for ${echo} yet. Deploy the echo realm first.`, true);
  } else {
    el("toast-title").textContent = trigger.title;
    renderMessage();
  }
} catch (err) {
  say("message", `Cannot load the echo trigger from ${gnotif.server}: ${err.message}`, true);
}
el("enable").disabled = !canEnable(false, trigger);
try {
  await refresh();
  if (!on && saved?.push) {
    say("message", "This browser stopped receiving notifications. Turn them on again to resume.");
  }
} catch (err) {
  say("message", err.message, true);
}
// The visitor turned live updates on before: resume them.
if (saved?.live && trigger !== null) {
  el("live").checked = true;
  try {
    await startLive();
  } catch (err) {
    stopLive();
    say("message", errorText(err), true);
  }
}
