import { Gnotif, GnotifError, type GnotifNotification, type Trigger } from "gnotif";
import { type Adena, connect, findAdena, sendEcho, shortAddress as short, walletLabel } from "./adena.ts";
import { gnokeyCommand } from "./echo-tx.ts";
import { configFor } from "./config.ts";
import { canEnable, listenerMismatch, optinsAtLoad, optinsFor } from "./optin.ts";
import { addressArea } from "./address-area.ts";
import { type Settings, load, save } from "./settings.ts";
import { WAIT_MS, answeredSend, blockSummary, clockText, deliveredBody, details, logShown } from "./live-log.ts";

// The name of the config.ts entry to use, which build.ts defines.
declare const DEMO_NETWORK: string;

const { chain, echo, gnoweb, gnotif: options } = configFor(DEMO_NETWORK);
const gnotif = new Gnotif({ ...options, serviceWorker: "/sw.js", icon: "/icon.png" });

function el<T extends HTMLElement>(id: string, kind: new () => T): T {
  const found = document.getElementById(id);
  if (!(found instanceof kind)) {
    throw new Error(`index.html has no ${kind.name} with id "${id}"`);
  }
  return found;
}

const realmURL = gnoweb.replace(/\/$/, "") + echo.replace(/^gno\.land/, "");
let adena: Adena | undefined;
let adenaState: "pending" | "found" | "absent" = "pending";
/** The connected Adena address, "" when none. */
let wallet = "";
let trigger: Trigger | null = null;
let on = false; // push is on
let live = false;

function stored(): Settings | null {
  return load(localStorage);
}

// remember saves the typed address and changes, and keeps the rest of the
// stored record.
function remember(changes: Partial<Settings>): void {
  const s = stored() ?? { address: "", optins: [], live: false, push: false };
  save(localStorage, { ...s, address: address(), ...changes });
}

function address(): string {
  return el("address", HTMLInputElement).value.trim();
}

function message(): string {
  const msg = el("msg", HTMLInputElement);
  const typed = msg.value.trim();
  return typed !== "" ? typed : msg.placeholder;
}

function say(id: string, text: string, isError = false): void {
  const line = el(id, HTMLElement);
  line.textContent = text;
  line.classList.toggle("error", isError);
}

function messageOf(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function errorText(err: unknown): string {
  if (err instanceof GnotifError && err.code === "denied") {
    return Notification.permission === "denied"
      ? "Notifications are blocked for this site. Allow them in the browser's site settings, then try again."
      : "The browser did not get permission. Try again and allow notifications.";
  }
  return messageOf(err);
}

function optin(): { trigger: string; value: string }[] {
  return optinsFor(trigger, address());
}

async function setOptin(): Promise<void> {
  const optins = optin();
  await gnotif.setOptins(optins);
  remember({ optins });
}

function setLiveState(text: string, connected = false): void {
  el("live-state", HTMLElement).classList.toggle("on", connected);
  el("live-state-text", HTMLElement).textContent = text;
}

function renderLog(): void {
  el("arrivals", HTMLElement).hidden = !logShown(live, el("arrivals-list", HTMLOListElement).children.length);
}

async function startLive(): Promise<void> {
  setLiveState("Connecting…");
  await setOptin();
  await gnotif.connect();
  live = true;
  setLiveState("Live updates on", true);
  renderLog();
}

function stopLive(): void {
  gnotif.disconnect();
  live = false;
  el("live", HTMLInputElement).checked = false;
  setLiveState("Live updates off");
  for (const sent of waiting) {
    clearInterval(sent.clock);
  }
  waiting.length = 0;
  el("arrivals-list", HTMLOListElement).replaceChildren();
  renderLog();
}

const ARRIVALS_KEPT = 5;

type Row = { item: HTMLLIElement; time: HTMLTimeElement; note: HTMLElement };

type Sent = { row: Row; body: string; at: number; clock: ReturnType<typeof setInterval>; late?: boolean };

/** Sends waiting for their live arrival, oldest first. */
const waiting: Sent[] = [];

function stamp(time: HTMLTimeElement, at: Date): void {
  time.dateTime = at.toISOString();
  time.textContent = at.toLocaleTimeString();
}

// logRow puts a row on top of the log and drops rows past ARRIVALS_KEPT; a
// dropped waiting row stops waiting.
function logRow(title: string, body: string, at: Date): Row {
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
  const list = el("arrivals-list", HTMLOListElement);
  list.prepend(item);
  for (const gone of Array.from(list.children).slice(ARRIVALS_KEPT)) {
    gone.remove();
    const i = waiting.findIndex((sent) => sent.row.item === gone);
    if (i !== -1) {
      const [dropped] = waiting.splice(i, 1);
      clearInterval(dropped?.clock);
    }
  }
  return { item, time, note };
}

function addSent(body: string): void {
  const at = new Date();
  const row = logRow(el("toast-title", HTMLElement).textContent, body, at);
  row.item.classList.add("pending");
  const dot = document.createElement("i");
  dot.className = "indicator";
  const clock = document.createTextNode(clockText(0));
  // Hidden from screen readers: the log would read the clock out every second.
  const ticking = document.createElement("span");
  ticking.setAttribute("aria-hidden", "true");
  ticking.append(dot, clock);
  row.time.replaceChildren(ticking);
  row.note.textContent = "Waiting for it to arrive";
  const tick = (): void => {
    clock.data = clockText(Date.now() - at.getTime());
  };
  const sent: Sent = { row, body, at: at.getTime(), clock: setInterval(tick, 1000) };
  waiting.push(sent);
  setTimeout(() => {
    if (waiting.includes(sent)) {
      sent.late = true;
      clearInterval(sent.clock);
      clock.data = clockText(WAIT_MS);
      row.item.classList.replace("pending", "late");
      row.note.textContent = `It has not arrived after ${WAIT_MS / 1000} s. Check that Adena sends from the address in step 1.`;
    }
  }, WAIT_MS);
}

function addArrival(n: GnotifNotification): void {
  const at = new Date();
  const i = answeredSend(waiting, n.body);
  const [sent] = i === -1 ? [] : waiting.splice(i, 1);
  let row: Row;
  let wait: number | undefined;
  if (sent === undefined) {
    row = logRow(n.title, n.body, at);
  } else {
    clearInterval(sent.clock);
    row = sent.row;
    row.item.classList.remove("pending", "late");
    stamp(row.time, at);
    wait = at.getTime() - sent.at;
  }
  row.note.replaceWith(arrivalDetails(n, wait));
  renderLog();
}

function arrivalDetails(n: GnotifNotification, wait: number | undefined): HTMLDetailsElement {
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

function pointToLog(): void {
  const link = document.createElement("a");
  link.href = "#arrivals";
  link.textContent = "Watch for it above ↑";
  el("sent", HTMLElement).classList.remove("error");
  el("sent", HTMLElement).replaceChildren("Sent. ", link);
}

// One amber button at a time: the next action in the flow.
function setPrimary(): void {
  el("enable", HTMLButtonElement).classList.toggle("primary", !on);
  el("send", HTMLButtonElement).classList.toggle("primary", on && adena !== undefined);
  el("copy", HTMLButtonElement).classList.toggle("primary", on && adena === undefined);
}

async function refresh(): Promise<void> {
  on = await gnotif.enabled();
  const saved = stored();
  const blocked = "Notification" in globalThis && Notification.permission === "denied";
  el("state", HTMLElement).classList.toggle("on", on);
  el("state-text", HTMLElement).textContent =
    on && saved !== null
      ? `On for ${saved.address}`
      : on
        ? "On"
        : blocked
          ? "Blocked in the browser's site settings"
          : "Off";
  el("enable", HTMLButtonElement).textContent =
    saved?.push === true && !on ? "Turn notifications on again" : "Turn on notifications";
  el("enable", HTMLButtonElement).hidden = on;
  el("disable", HTMLButtonElement).hidden = !on;
  renderAddress();
  setPrimary();
}

function renderAddress(): void {
  const area = addressArea({ on, adena: adenaState, wallet });
  el("address-row", HTMLElement).hidden = area.input === "hidden";
  el("address", HTMLInputElement).readOnly = area.input === "readonly";
  el("connect", HTMLButtonElement).hidden = !area.connect;
  el("wallet", HTMLElement).hidden = !area.wallet;
  el("wallet-address", HTMLElement).textContent = wallet;
  el("disconnect", HTMLButtonElement).hidden = !area.disconnect;
  el("get-adena", HTMLElement).hidden = !area.getAdena;
  el("address-hint", HTMLElement).hidden = area.locked;
  el("locked-hint", HTMLElement).hidden = !area.locked;
}

function busy(state: boolean): void {
  for (const id of ["disable", "connect", "disconnect", "send"]) {
    el(id, HTMLButtonElement).disabled = state;
  }
  el("enable", HTMLButtonElement).disabled = !canEnable(state, trigger);
}

function noteMismatch(caller: string): void {
  const listening = listenerMismatch(caller, on, stored());
  say(
    "mismatch",
    listening === undefined
      ? ""
      : `Adena sends from ${short(caller)}, but this browser listens for ${short(listening)}. The notification goes to ${short(caller)}.`,
  );
}

function notificationBody(msg: string): string {
  return trigger !== null ? trigger.body.replaceAll("{msg}", msg) : msg;
}

function renderMessage(): void {
  const msg = message();
  el("toast-body", HTMLElement).textContent = notificationBody(msg);
  el("command", HTMLElement).textContent = gnokeyCommand(chain, echo, msg);
}

async function turnOn(): Promise<void> {
  try {
    optin();
  } catch (err) {
    say("message", messageOf(err), true);
    el("address", HTMLInputElement).focus();
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
}

el("enable", HTMLButtonElement).addEventListener("click", () => void turnOn());

async function turnOff(): Promise<void> {
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
}

el("disable", HTMLButtonElement).addEventListener("click", () => void turnOff());

async function connectAdena(): Promise<void> {
  // The button shows only once findAdena has found the wallet.
  if (adena === undefined) {
    return;
  }
  const before = el("connect", HTMLButtonElement).textContent;
  busy(true);
  el("connect", HTMLButtonElement).textContent = walletLabel("pending");
  say("message", "Waiting for Adena…");
  try {
    wallet = await connect(adena, chain);
    el("address", HTMLInputElement).value = wallet;
    say("message", `Adena connected as ${short(wallet)}.`);
    say("mismatch", "");
  } catch (err) {
    say("message", messageOf(err), true);
  }
  el("connect", HTMLButtonElement).textContent = before;
  busy(false);
  renderAddress();
}

el("connect", HTMLButtonElement).addEventListener("click", () => void connectAdena());

el("disconnect", HTMLButtonElement).addEventListener("click", () => {
  wallet = "";
  say("message", "Adena disconnected. Type an address, or connect again.");
  renderAddress();
});

async function send(): Promise<void> {
  // The button shows only once findAdena has found the wallet.
  if (adena === undefined) {
    return;
  }
  if (el("msg", HTMLInputElement).value.trim() === "") {
    say("sent", "Write a message first.", true);
    el("msg", HTMLInputElement).focus();
    return;
  }
  const before = el("send", HTMLButtonElement).textContent;
  busy(true);
  el("send", HTMLButtonElement).textContent = walletLabel("pending");
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
      say(
        "sent",
        on
          ? `Sent. The notification arrives in a few seconds.${hash === undefined || hash === "" ? "" : ` Transaction ${hash}.`}`
          : "Sent, but notifications are off in this browser. Turn them on in step 1 to get the next one.",
      );
    }
  } catch (err) {
    say("sent", messageOf(err), true);
  }
  el("send", HTMLButtonElement).textContent = before;
  busy(false);
}

el("send", HTMLButtonElement).addEventListener("click", () => void send());

async function copyCommand(): Promise<void> {
  try {
    await navigator.clipboard.writeText(el("command", HTMLElement).textContent);
    el("copy", HTMLButtonElement).textContent = "Copied";
    setTimeout(() => {
      el("copy", HTMLButtonElement).textContent = "Copy";
    }, 1500);
  } catch {
    say("sent", "Copying failed. Select the command and copy it yourself.", true);
  }
}

el("copy", HTMLButtonElement).addEventListener("click", () => void copyCommand());

el("msg", HTMLInputElement).addEventListener("input", renderMessage);

async function switchLive(): Promise<void> {
  if (!el("live", HTMLInputElement).checked) {
    stopLive();
    remember({ live: false });
    say("message", "Live updates off.");
    return;
  }
  try {
    optin();
  } catch (err) {
    el("live", HTMLInputElement).checked = false;
    say("message", messageOf(err), true);
    el("address", HTMLInputElement).focus();
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
}

el("live", HTMLInputElement).addEventListener("change", () => void switchLive());

// While live updates are on, a new address replaces the opt-in at once.
async function changeAddress(): Promise<void> {
  if (!live) {
    return;
  }
  try {
    await setOptin();
  } catch (err) {
    say("message", errorText(err), true);
  }
}

el("address", HTMLInputElement).addEventListener("change", () => void changeAddress());

gnotif.on("notification", addArrival);
gnotif.on("reconnecting", () => setLiveState("Reconnecting…"));
gnotif.on("reconnect", () => setLiveState("Live updates on", true));
gnotif.on("error", (err) => say("message", err.message, true));

el("chain-name", HTMLElement).textContent = chain.name;
el("chain-id", HTMLElement).textContent = chain.id;
el("realm-link", HTMLAnchorElement).href = realmURL;
el("realm-link-foot", HTMLAnchorElement).href = realmURL;
el("server", HTMLElement).textContent = gnotif.server;
el("toast-host", HTMLElement).textContent = location.host;
el("code-realm", HTMLElement).textContent = echo;
void findAdena(globalThis).then((found) => {
  adena = found;
  adenaState = found === undefined ? "absent" : "found";
  el("send-row", HTMLElement).hidden = found === undefined;
  renderAddress();
  setPrimary();
});
renderMessage();

const saved = stored();
if (saved !== null) {
  el("address", HTMLInputElement).value = saved.address;
}
try {
  [trigger = null] = await gnotif.triggers(echo);
  if (trigger === null) {
    say("message", `${gnotif.server} lists no trigger for ${echo} yet. Deploy the echo realm first.`, true);
  } else {
    el("toast-title", HTMLElement).textContent = trigger.title;
    renderMessage();
  }
} catch (err) {
  say("message", `Cannot load the echo trigger from ${gnotif.server}: ${messageOf(err)}`, true);
}
el("enable", HTMLButtonElement).disabled = !canEnable(false, trigger);
try {
  await refresh();
  if (!on && saved?.push === true) {
    say("message", "This browser stopped receiving notifications. Turn them on again to resume.");
  }
  if (optinsAtLoad(on, saved?.live === true, trigger)) {
    await setOptin();
  }
} catch (err) {
  say("message", messageOf(err), true);
}
// The visitor turned live updates on before: resume them.
if (saved?.live === true && trigger !== null) {
  el("live", HTMLInputElement).checked = true;
  try {
    await startLive();
  } catch (err) {
    stopLive();
    say("message", errorText(err), true);
  }
}
