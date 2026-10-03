import { Gnotif } from "./gnotif.js";

const gnotif = new Gnotif({ server: location.origin, serviceWorker: "/sw.js" });
const el = (id) => document.getElementById(id);
let triggers = [];
let enabled = false;

function status(text, isError = false) {
  el("status").textContent = text;
  el("status").classList.toggle("error", isError);
}

function renderTriggers() {
  el("triggers").replaceChildren(...triggers.map((t) => {
    const box = document.createElement("input");
    box.type = "checkbox";
    box.dataset.id = t.id;
    box.addEventListener("change", () => {
      if (enabled) {
        sync();
      }
    });
    const title = document.createElement("strong");
    title.textContent = t.title;
    const target = document.createElement("span");
    target.className = "target";
    target.textContent = `${t.event} on ${t.target}`;
    const label = document.createElement("label");
    label.append(box, title, target);
    if (t.verified) {
      const mark = document.createElement("span");
      mark.className = "verified";
      mark.textContent = "✓ verified";
      label.append(mark);
    }
    const item = document.createElement("li");
    item.append(label);
    return item;
  }));
  if (triggers.length === 0) {
    const item = document.createElement("li");
    item.textContent = "No trigger declared yet.";
    el("triggers").append(item);
  }
}

// selectedOptins throws when a checked trigger needs the address and it is empty.
function selectedOptins() {
  const address = el("address").value.trim();
  return [...el("triggers").querySelectorAll("input:checked")].map((box) => {
    const t = triggers.find((x) => x.id === box.dataset.id);
    if (t.param && !address) {
      throw new Error(`Enter your address: "${t.title}" needs it.`);
    }
    return { trigger: t.id, value: t.param ? address : "" };
  });
}

async function sync() {
  try {
    const optins = selectedOptins();
    await gnotif.setOptins(optins);
    status(`Notifications on for ${optins.length} trigger${optins.length === 1 ? "" : "s"}.`);
  } catch (err) {
    status(err.message, true);
  }
}

el("enable").addEventListener("click", async () => {
  let optins;
  try {
    optins = selectedOptins();
  } catch (err) {
    status(err.message, true);
    return;
  }
  try {
    status("Asking for permission…");
    await gnotif.enable();
    enabled = true;
    await gnotif.setOptins(optins);
    status(`Notifications on for ${optins.length} trigger${optins.length === 1 ? "" : "s"}.`);
  } catch (err) {
    status(err.message, true);
  }
});

el("disable").addEventListener("click", async () => {
  try {
    await gnotif.disable();
    enabled = false;
    status("Notifications off.");
  } catch (err) {
    status(err.message, true);
  }
});

try {
  triggers = await gnotif.triggers();
  renderTriggers();
  const game = new URLSearchParams(location.search).get("game");
  status(game ? `Opened from game ${game}.` : `Permission: ${globalThis.Notification?.permission ?? "unavailable"}.`);
} catch (err) {
  status(`Cannot load triggers: ${err.message}`, true);
}
