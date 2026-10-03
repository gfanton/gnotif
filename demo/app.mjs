import { Gnotif, GnotifError } from "./gnotif.js";
import { gnoweb, pingpong, server } from "./config.js";

const gnotif = new Gnotif({ server, serviceWorker: "/sw.js" });
const el = (id) => document.getElementById(id);
// The server cannot read opt-ins back, so the page keeps the last ones it set.
const STORAGE_KEY = "gnotif-demo";
const realmURL = gnoweb.replace(/\/$/, "") + pingpong.replace(/^gno\.land/, "");
let triggers = [];
let on = false;
let pending = Promise.resolve();

/** @returns {{ address: string, optins: { trigger: string, value: string }[] } | null} */
function stored() {
  return JSON.parse(localStorage.getItem(STORAGE_KEY) ?? "null");
}

function save(optins) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify({ address: el("address").value.trim(), optins }));
}

function element(tag, className, text) {
  const e = document.createElement(tag);
  e.className = className;
  if (text !== undefined) {
    e.textContent = text;
  }
  return e;
}

function plural(n) {
  return `${n} trigger${n === 1 ? "" : "s"}`;
}

function say(text, isError = false) {
  el("message").textContent = text;
  el("message").classList.toggle("error", isError);
}

function errorText(err) {
  if (err instanceof GnotifError && err.code === "denied") {
    return Notification.permission === "denied"
      ? "Notifications are blocked for this site. Allow them in the browser's site settings, then try again."
      : "The browser did not get permission. Try again and allow notifications.";
  }
  return err.message;
}

function featured(t) {
  return t.target === pingpong && t.verified;
}

function mark(verified) {
  const m = element("span", verified ? "mark verified" : "mark", verified ? "Verified" : "Not verified");
  if (verified) {
    m.prepend(el("icon-verified").content.cloneNode(true));
  }
  return m;
}

// preview draws the notification a trigger sends, with its {key} placeholders.
function preview(t) {
  const body = element("p", "preview-body");
  for (const part of t.body.split(/(\{\w+\})/)) {
    body.append(/^\{\w+\}$/.test(part) ? element("code", "", part) : part);
  }
  const figure = element("figure", "preview");
  figure.setAttribute("aria-label", "Notification preview");
  figure.append(element("p", "preview-origin", location.host), element("p", "preview-title", t.title), body);
  return figure;
}

function renderTriggers(saved) {
  if (triggers.length === 0) {
    el("triggers").replaceChildren(element("li", "note", "No notifications on offer yet. pingpong declares its trigger with DeclareTriggers."));
    return;
  }
  el("triggers").replaceChildren(...triggers.map((t) => {
    const box = document.createElement("input");
    box.type = "checkbox";
    box.value = t.id;
    box.checked = saved ? saved.optins.some((o) => o.trigger === t.id) : featured(t);
    box.addEventListener("change", () => {
      if (on) {
        sync();
      }
    });
    const head = element("span", "trigger-head");
    head.append(element("span", "trigger-title", t.title), mark(t.verified));
    const label = element("label", "trigger");
    label.append(box, head, element("span", "trigger-meta", `${t.event} on ${t.target}`));
    const item = document.createElement("li");
    item.append(label);
    if (featured(t)) {
      item.append(preview(t));
    }
    return item;
  }));
}

// selectedOptins throws when a checked trigger needs the address and it is empty.
function selectedOptins() {
  const address = el("address").value.trim();
  return [...el("triggers").querySelectorAll("input:checked")].map((box) => {
    const t = triggers.find((x) => x.id === box.value);
    if (t.param && !address) {
      throw new Error(`Enter your address: "${t.title}" needs it.`);
    }
    return { trigger: t.id, value: t.param ? address : "" };
  });
}

async function setOptins(optins) {
  await gnotif.setOptins(optins);
  save(optins);
  say(`Notifications on for ${plural(optins.length)}.`);
}

async function refresh() {
  on = await gnotif.enabled();
  const saved = stored();
  el("permission").textContent = "Notification" in globalThis
    ? { granted: "Granted", denied: "Blocked", default: "Not granted" }[Notification.permission]
    : "Not supported";
  el("subscribed").textContent = on ? "Yes" : "No";
  el("following").textContent = !on ? "None" : saved ? plural(saved.optins.length) : "Unknown";
  el("switch-hint").textContent = on
    ? "Notifications are on. Changes to your address or choices save as you make them."
    : "Your browser asks for permission once. Notifications then arrive even with this tab closed.";
  el("enable").textContent = saved ? "Turn notifications on again" : "Turn on notifications";
  el("enable").hidden = on;
  el("disable").hidden = !on;
}

// Opt-in changes run one at a time, so the last change is the one the server keeps.
function sync() {
  pending = pending.then(async () => {
    try {
      await setOptins(selectedOptins());
    } catch (err) {
      say(errorText(err), true);
    }
    await refresh();
  }).catch((err) => say(err.message, true));
}

function busy(state) {
  el("enable").disabled = state;
  el("disable").disabled = state;
}

el("enable").addEventListener("click", async () => {
  let optins;
  try {
    optins = selectedOptins();
  } catch (err) {
    say(err.message, true);
    el("address").focus();
    return;
  }
  busy(true);
  say("Turning on notifications…");
  try {
    await gnotif.enable();
    await setOptins(optins);
  } catch (err) {
    say(errorText(err), true);
  }
  busy(false);
  await refresh();
});

el("disable").addEventListener("click", async () => {
  busy(true);
  try {
    await gnotif.disable();
    localStorage.removeItem(STORAGE_KEY);
    say("Notifications off.");
  } catch (err) {
    say(errorText(err), true);
  }
  busy(false);
  await refresh();
});

el("address").addEventListener("change", () => {
  if (on) {
    sync();
  }
});

const game = new URLSearchParams(location.search).get("game");
if (game) {
  el("turn-game").textContent = game;
  el("turn-link-game").textContent = game;
  el("turn-link").href = `${realmURL}:game/${encodeURIComponent(game)}`;
  el("turn").hidden = false;
}
el("realm-link").href = realmURL;
el("server").textContent = server;

const saved = stored();
if (saved) {
  el("address").value = saved.address;
}
try {
  triggers = (await gnotif.triggers()).sort((a, b) => Number(featured(b)) - Number(featured(a)));
  renderTriggers(saved);
} catch (err) {
  el("triggers").replaceChildren(element("li", "note error", `Cannot load notifications from ${server}: ${err.message}`));
}
try {
  await refresh();
  if (!on && saved) {
    say("This browser stopped receiving notifications. Turn them on again to resume.");
  }
} catch (err) {
  say(err.message, true);
}
