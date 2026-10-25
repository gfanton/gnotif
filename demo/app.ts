import { Gnotif, GnotifError, type GnotifNotification, type Trigger } from "gnotif";
import { type Adena, connect, findAdena, sendEcho, shortAddress as short, walletLabel } from "./adena.ts";
import { commandWords, gnokeyCommand } from "./echo-tx.ts";
import { configFor } from "./config.ts";
import { addressProblem } from "./address.ts";
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
import { addressArea, lockedRefusal } from "./address-area.ts";
import { sendArea } from "./send-area.ts";
import { type Settings, load, save } from "./settings.ts";
import {
  type SentWith,
  answeredSend,
  arrivedText,
  bannerCheck,
  blockSummary,
  bodyFor,
  details,
  lateAfter,
  lateNote,
  lateRowNote,
  logShown,
  sentText,
  waitOf,
  waitsFor,
} from "./live-log.ts";

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
let working = false;

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
  return typed !== "" ? typed : "Hello, me";
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
  const permission = "Notification" in globalThis ? Notification.permission : undefined;
  return pushErrorText(err instanceof GnotifError ? err.code : undefined, permission, messageOf(err));
}

// A busy control takes aria-disabled, not disabled, so it keeps focus; its
// handler checks `working`.
function markBusy(button: HTMLButtonElement, state: boolean): void {
  if (state) {
    button.setAttribute("aria-disabled", "true");
  } else {
    button.removeAttribute("aria-disabled");
  }
}

// refocus moves focus to next when the control that had it went away.
function refocus(next: HTMLElement): void {
  const active = document.activeElement;
  if (
    active === null ||
    active === document.body ||
    (active instanceof HTMLElement && active.closest("[hidden]") !== null)
  ) {
    next.focus();
  }
}

function optin(): { trigger: string; value: string }[] {
  return optinsFor(trigger, address());
}

async function setOptin(): Promise<void> {
  const optins = optin();
  await gnotif.setOptins(optins);
  remember({ optins });
  setPrimary();
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
  setPrimary();
}

function stopLive(): void {
  gnotif.disconnect();
  live = false;
  el("live", HTMLInputElement).checked = false;
  setLiveState("Live updates off");
  // With notifications on, a push still brings each waiting send.
  if (!on) {
    waiting.length = 0;
    lastSent = undefined;
    el("arrivals-list", HTMLOListElement).replaceChildren();
    say("sent", "");
    say("banner-check", "");
  }
  renderLog();
  setPrimary();
}

const ARRIVALS_KEPT = 5;

type Row = { item: HTMLLIElement; time: HTMLTimeElement; note: HTMLElement };

type Sent = {
  row: Row;
  body: string;
  via: SentWith;
  at: number;
  late?: boolean;
};

/** Sends waiting for their arrival, oldest first. */
const waiting: Sent[] = [];
/** The send step 2's line reports on: the latest one. */
let lastSent: Sent | undefined;

// renderPreview shows the notification the next send becomes.
function renderPreview(): void {
  el("toast-title", HTMLElement).textContent = trigger?.title ?? "Echo";
  el("toast-body", HTMLElement).textContent = bodyFor(trigger?.body, message());
}

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
  const status = document.createElement("div");
  status.className = "row-status";
  status.append(note);
  const item = document.createElement("li");
  item.className = "arrival";
  item.append(time, head, text, status);
  const list = el("arrivals-list", HTMLOListElement);
  list.prepend(item);
  for (const gone of Array.from(list.children).slice(ARRIVALS_KEPT)) {
    gone.remove();
    const i = waiting.findIndex((sent) => sent.row.item === gone);
    if (i !== -1) {
      waiting.splice(i, 1);
    }
  }
  return { item, time, note };
}

function addSent(body: string, via: SentWith): void {
  const at = new Date();
  const row = logRow(el("toast-title", HTMLElement).textContent, body, at);
  row.item.classList.add("pending");
  row.note.textContent = "Waiting for it to arrive";
  const sent: Sent = { row, body, via, at: at.getTime() };
  waiting.push(sent);
  lastSent = sent;
  say("banner-check", "");
  renderLog();
  setTimeout(() => {
    if (waiting.includes(sent)) {
      sent.late = true;
      row.item.classList.replace("pending", "late");
      row.note.textContent = lateRowNote(via);
      if (sent === lastSent) {
        say("sent", lateNote(via), true);
      }
    }
  }, lateAfter(via));
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
    row = sent.row;
    row.item.classList.remove("pending", "late");
    stamp(row.time, at);
    wait = waitOf(sent, at.getTime());
    if (sent === lastSent) {
      say("sent", "");
      say("banner-check", bannerCheck(n.source));
    }
  }
  row.note.replaceWith(arrivalDetails(n, wait));
  renderLog();
}

function arrivalDetails(n: GnotifNotification, wait: number | undefined): HTMLDetailsElement {
  // The row's lit dot shows the arrival; screen readers hear it here.
  const heard = document.createElement("span");
  heard.className = "visually-hidden";
  heard.textContent = `${arrivedText(wait, n.body)}. `;
  const summary = document.createElement("summary");
  summary.append(heard, blockSummary(n.height, wait));
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

function readyToSend(): boolean {
  return canSend(trigger, address(), on || live, stored()?.optins ?? []);
}

function pushAccess(): { supported: boolean; blocked: boolean } {
  return {
    supported: pushSupported(globalThis),
    blocked: "Notification" in globalThis && Notification.permission === "denied",
  };
}

/** The lead last shown in step 2: true for gnokey, undefined before the first render. */
let gnokeyLed: boolean | undefined;

function adenaSends(): boolean {
  return sendArea({ adena: adenaState, wallet, address: address() }).adenaSend === "enabled";
}

// One amber button at a time: the next action in the flow.
function setPrimary(): void {
  const ready = readyToSend();
  const area = sendArea({ adena: adenaState, wallet, address: address() });
  el("enable", HTMLButtonElement).classList.toggle("primary", turnOnIsNext({ on, live, ...pushAccess() }));
  el("send", HTMLButtonElement).hidden = area.adenaSend === "hidden";
  el("send", HTMLButtonElement).classList.toggle("primary", ready && area.adenaSend === "enabled");
  el("copy-lead", HTMLButtonElement).hidden = !area.gnokeyLeads;
  el("copy-lead", HTMLButtonElement).classList.toggle("primary", ready && area.gnokeyLeads);
  for (const id of ["send", "copy-lead", "copy"]) {
    el(id, HTMLButtonElement).disabled = !ready || (id === "send" && area.adenaSend !== "enabled");
  }
  // The gnokey details follow a change of lead only, so a visitor who closed
  // them while typing keeps them closed. They wait for the look for Adena,
  // so they never open and then close for a visitor who has it.
  if (adenaState !== "pending" && area.gnokeyLeads !== gnokeyLed) {
    gnokeyLed = area.gnokeyLeads;
    el("cli", HTMLDetailsElement).open = area.gnokeyLeads;
    el("cli-label", HTMLElement).textContent = area.gnokeyLeads ? "Send with gnokey" : "Use gnokey instead";
  }
  const text = sendHint(address(), ready, trigger);
  for (const id of ["send-hint", "copy-hint"]) {
    const hint = el(id, HTMLElement);
    // When gnokey leads, step 2's own hint already stands for the command.
    hint.hidden = ready || (id === "copy-hint" && area.gnokeyLeads);
    if (hint.textContent !== text) {
      hint.textContent = text;
    }
  }
}

async function refresh(): Promise<void> {
  on = await gnotif.enabled();
  const saved = stored();
  const { supported, blocked } = pushAccess();
  el("state", HTMLElement).classList.toggle("on", on);
  el("state-text", HTMLElement).textContent = notificationState({
    supported,
    on,
    blocked,
    address: saved?.address ?? "",
  });
  el("enable", HTMLButtonElement).textContent = pushStopped({ wasOn: saved?.push === true, on, supported, blocked })
    ? "Turn notifications on again"
    : "Turn on notifications";
  el("enable", HTMLButtonElement).hidden = on;
  el("enable", HTMLButtonElement).disabled = !supported || !canEnable(false, trigger);
  el("push-hint", HTMLElement).hidden = !supported;
  if (!supported) {
    say("message", pushHelp(supported));
  }
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
  setPrimary();
}

function busy(state: boolean): void {
  working = state;
  for (const id of ["enable", "disable", "connect", "disconnect", "send", "copy-lead", "copy"]) {
    markBusy(el(id, HTMLButtonElement), state);
  }
  setPrimary();
}

function renderMessage(): void {
  const msg = message();
  renderPreview();
  el("command", HTMLElement).replaceChildren(
    ...commandWords(gnokeyCommand(chain, echo, msg)).map(({ text, word }) => {
      if (!word) {
        return text;
      }
      const span = document.createElement("span");
      span.className = "nowrap";
      span.textContent = text;
      return span;
    }),
  );
}

// showAddressError puts step 1's message about the address under the field;
// "" clears it.
function showAddressError(text: string): void {
  el("address-error", HTMLElement).textContent = text;
  const input = el("address", HTMLInputElement);
  if (text === "") {
    input.removeAttribute("aria-invalid");
  } else {
    input.setAttribute("aria-invalid", "true");
  }
}

// addressReady shows what keeps the typed address from being used, and
// focuses the field when there is something to fix.
function addressReady(): boolean {
  const text = addressMessage(address());
  showAddressError(text);
  if (text !== "") {
    el("address", HTMLInputElement).focus();
  }
  return text === "";
}

async function turnOn(): Promise<void> {
  if (on || !canEnable(working, trigger) || !addressReady()) {
    return;
  }
  try {
    optin();
  } catch (err) {
    say("message", messageOf(err), true);
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
  refocus(el("disable", HTMLButtonElement));
}

el("setup", HTMLFormElement).addEventListener("submit", (event) => {
  event.preventDefault();
  void turnOn();
});
// Enter in the address field turns notifications on only while that is step
// 1's lead action, not while live updates already receive.
el("address", HTMLInputElement).addEventListener("keydown", (event) => {
  if (event.key === "Enter" && !event.isComposing && !turnOnIsNext({ on, live, ...pushAccess() })) {
    event.preventDefault();
  }
});

async function turnOff(): Promise<void> {
  if (working) {
    return;
  }
  busy(true);
  try {
    await gnotif.disable();
    remember({ push: false });
    say("message", "");
  } catch (err) {
    say("message", errorText(err), true);
  }
  busy(false);
  await refresh();
  refocus(el("enable", HTMLButtonElement));
}

el("disable", HTMLButtonElement).addEventListener("click", () => void turnOff());

async function connectAdena(): Promise<void> {
  if (adena === undefined || working) {
    return;
  }
  const before = el("connect", HTMLButtonElement).textContent;
  busy(true);
  el("connect", HTMLButtonElement).textContent = walletLabel("pending");
  say("wallet-message", "Waiting for Adena…");
  try {
    const account = await connect(adena, chain);
    const refusal = lockedRefusal(on, address(), account);
    if (refusal === "") {
      wallet = account;
      el("address", HTMLInputElement).value = wallet;
      showAddressError("");
      say("wallet-message", `Adena connected as ${short(wallet)}.`);
      await changeAddress();
    } else {
      say("wallet-message", refusal, true);
    }
  } catch (err) {
    say("wallet-message", messageOf(err), true);
  }
  el("connect", HTMLButtonElement).textContent = before;
  busy(false);
  renderAddress();
  // While notifications are on, Disconnect stays hidden: the next action is
  // step 2's.
  refocus(el(on ? "msg" : "disconnect", HTMLElement));
}

el("connect", HTMLButtonElement).addEventListener("click", () => void connectAdena());

el("disconnect", HTMLButtonElement).addEventListener("click", () => {
  if (working) {
    return;
  }
  wallet = "";
  say("wallet-message", "Adena disconnected. Type an address, or connect again.");
  renderAddress();
  refocus(el("address", HTMLInputElement));
});

async function send(): Promise<void> {
  if (adena === undefined || working || !readyToSend() || !adenaSends()) {
    return;
  }
  const before = el("send", HTMLButtonElement).textContent;
  busy(true);
  el("send", HTMLButtonElement).textContent = walletLabel("pending");
  say("sent", "Waiting for Adena…");
  try {
    const msg = message();
    const { hash } = await sendEcho(adena, chain, echo, msg, address());
    addSent(bodyFor(trigger?.body, msg), "adena");
    say("sent", sentText("adena", hash));
  } catch (err) {
    say("sent", messageOf(err), true);
  }
  el("send", HTMLButtonElement).textContent = before;
  busy(false);
}

el("compose", HTMLFormElement).addEventListener("submit", (event) => {
  event.preventDefault();
  void send();
});

function showCopied(button: HTMLButtonElement): void {
  button.dataset["label"] ??= button.textContent;
  button.textContent = "Copied";
  setTimeout(() => {
    button.textContent = button.dataset["label"] ?? button.textContent;
  }, 1500);
}

// copyCommand copies the gnokey command and reads "Copied" on button for a
// moment.
async function copyCommand(button: HTMLButtonElement): Promise<void> {
  if (working || !readyToSend()) return;
  try {
    await navigator.clipboard.writeText(el("command", HTMLElement).textContent);
  } catch {
    say("sent", "Copying failed. Select the command and copy it yourself.", true);
    return;
  }
  showCopied(button);
  const body = bodyFor(trigger?.body, message());
  if (!waitsFor(waiting, body)) {
    addSent(body, "gnokey");
  }
  say("sent", sentText("gnokey", undefined));
}

for (const id of ["copy", "copy-lead"]) {
  const button = el(id, HTMLButtonElement);
  button.addEventListener("click", () => void copyCommand(button));
}

// A code reveal's Copy copies its code; where the clipboard refuses, it
// selects the code for the visitor to copy.
for (const button of document.querySelectorAll<HTMLButtonElement>("button[data-copy]")) {
  const code = el(button.dataset["copy"] ?? "", HTMLElement);
  button.addEventListener("click", () => {
    void navigator.clipboard.writeText(code.textContent).then(
      () => showCopied(button),
      () => getSelection()?.selectAllChildren(code),
    );
  });
}

el("msg", HTMLInputElement).addEventListener("input", () => renderMessage());
// Without Adena, copying the command is step 2's lead action, so Enter runs it.
el("msg", HTMLInputElement).addEventListener("keydown", (event) => {
  if (event.key !== "Enter" || event.isComposing || adenaSends()) {
    return;
  }
  event.preventDefault();
  void copyCommand(el("copy-lead", HTMLButtonElement));
});

async function switchLive(): Promise<void> {
  if (!el("live", HTMLInputElement).checked) {
    stopLive();
    remember({ live: false });
    say("live-message", "");
    return;
  }
  if (!addressReady()) {
    el("live", HTMLInputElement).checked = false;
    return;
  }
  try {
    optin();
  } catch (err) {
    el("live", HTMLInputElement).checked = false;
    say("live-message", messageOf(err), true);
    return;
  }
  try {
    await startLive();
    remember({ live: true });
    say("live-message", "Live updates on. Send yourself a message in step 2.");
  } catch (err) {
    stopLive();
    say("live-message", errorText(err), true);
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
    say("live-message", errorText(err), true);
  }
  setPrimary();
}

// A fixed or cleared address drops its error while typing; a new one shows only
// once the field loses focus or an action needs the address.
el("address", HTMLInputElement).addEventListener("input", () => {
  const problem = addressProblem(address());
  if (problem === undefined || problem === "empty") {
    showAddressError("");
  }
  setPrimary();
});
el("address", HTMLInputElement).addEventListener("change", () => {
  showAddressError(address() === "" ? "" : addressMessage(address()));
  remember({});
  void changeAddress();
});

gnotif.on("notification", addArrival);
gnotif.on("reconnecting", () => {
  setLiveState("Reconnecting…");
  say("live-message", "The live connection dropped. Reconnecting…");
});
gnotif.on("reconnect", () => {
  setLiveState("Live updates on", true);
  say("live-message", "Live updates on again.");
});
gnotif.on("error", (err) => say("live-message", err.message, true));

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
  el("send-action", HTMLElement).hidden = false;
  renderAddress();
  setPrimary();
});
const saved = stored();
if (saved !== null) {
  el("address", HTMLInputElement).value = saved.address;
}
renderMessage();
setPrimary();

try {
  [trigger = null] = await gnotif.triggers(echo);
  if (trigger === null) {
    say(
      "page-message",
      `${gnotif.server} lists no trigger for ${echo} yet. If you run this demo yourself, deploy the echo realm first. Otherwise, try again later.`,
      true,
    );
  } else {
    renderMessage();
  }
} catch (err) {
  say(
    "page-message",
    `Cannot load the echo trigger from ${gnotif.server}: ${messageOf(err).replace(/\.+$/, "")}. Reload the page to try again.`,
    true,
  );
}
try {
  await refresh();
  const { supported, blocked } = pushAccess();
  say(
    "message",
    pushNoteAtLoad({
      on,
      supported,
      triggerLoaded: trigger !== null,
      stopped: pushStopped({ wasOn: saved?.push === true, on, supported, blocked }),
    }),
  );
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
    say("live-message", errorText(err), true);
  }
}
