// The live log's decisions, apart from the DOM.
import type { GnotifNotification } from "gnotif";

/**
 * answeredSend returns the index of the oldest send in waiting whose
 * notification has body, taking one still on time before a late one, or -1
 * when none waits for it.
 * @param waiting oldest first
 * @param body the arrival's notification body
 */
export function answeredSend(waiting: { body: string; late?: boolean }[], body: string): number {
  const onTime = waiting.findIndex((sent) => sent.body === body && sent.late !== true);
  return onTime !== -1 ? onTime : waiting.findIndex((sent) => sent.body === body);
}

/** MAX_BODY is gnotifd's limit on a notification body, in UTF-8 bytes. */
export const MAX_BODY = 255;

/**
 * deliveredBody is body as gnotifd delivers it: cut to MAX_BODY bytes of
 * UTF-8, at the start of a character.
 */
export function deliveredBody(body: string): string {
  const bytes = new TextEncoder().encode(body);
  if (bytes.length <= MAX_BODY) {
    return body;
  }
  let cut = MAX_BODY;
  while (cut > 0 && ((bytes[cut] ?? 0) & 0xc0) === 0x80) {
    cut--;
  }
  return new TextDecoder().decode(bytes.subarray(0, cut));
}

/**
 * blockSummary is an arrival's line in the log: its block, and how long it
 * took after its send when one waited for it.
 * @param wait milliseconds from the send to the arrival
 */
export function blockSummary(height: number, wait: number | undefined): string {
  return wait === undefined ? `Block ${height}` : `Block ${height} · ${waitText(wait)}`;
}

/**
 * details lists what an arrival carries besides its block, as label and value
 * pairs: the block's time as format writes it, the transaction, the channel
 * that delivered it, the trigger with its opted-in value, the event's
 * attributes and the link. A time that does not parse is left out.
 */
export function details(n: GnotifNotification, format: (at: Date) => string): [string, string][] {
  const at = new Date(n.time);
  const rows: [string, string][] = Number.isNaN(at.getTime()) ? [] : [["Block time", format(at)]];
  rows.push(
    ["Transaction", n.tx],
    ["Delivered by", n.source === "live" ? "live connection" : "push"],
    ["Trigger", n.value === "" ? n.trigger : `${n.trigger} for ${n.value}`],
  );
  for (const { key, value } of n.attrs) {
    rows.push([key, value]);
  }
  rows.push(["Link", n.link]);
  return rows;
}

/**
 * logShown reports whether the arrivals log shows: while live updates are on,
 * and while it lists a row, which a push brings with live updates off.
 * @param live whether live updates are on
 * @param rows the rows the log lists
 */
export function logShown(live: boolean, rows: number): boolean {
  return live || rows > 0;
}

/**
 * waitText words the wait from a send to its arrival in whole seconds, or as
 * within a second under 1 s. A no-break space keeps the number with its unit.
 * @param ms milliseconds from the send to the arrival
 */
export function waitText(ms: number): string {
  return ms < 1000 ? "Arrived within a second of sending" : `Arrived ${Math.round(ms / 1000)}\u00a0s after sending`;
}

/** SentWith is how the visitor sent an Echo: from the page with Adena, or from a terminal with gnokey. */
export type SentWith = "adena" | "gnokey";

// A gnokey send's clock starts at the copy, before the visitor pastes, edits
// and signs the command, so it waits longer before it reads as late.
const lateness = {
  adena: {
    after: 60_000,
    note: "It has not arrived after 60\u00a0s. Check that Adena sends from the address in step 1.",
    row: "Not arrived after 60\u00a0s",
  },
  gnokey: {
    after: 180_000,
    note: "It has not arrived after 3\u00a0min. If you ran the command, check that it used the key of the address in step 1, and that the address has test GNOT.",
    row: "Not arrived after 3\u00a0min",
  },
} as const satisfies Record<SentWith, { after: number; note: string; row: string }>;

/** lateAfter is how long a send waits for its arrival before it reads as late, in milliseconds. */
export function lateAfter(via: SentWith): number {
  return lateness[via].after;
}

/** lateNote is a send's line once lateAfter passes with no arrival: what to check for the way it was sent. */
export function lateNote(via: SentWith): string {
  return lateness[via].note;
}

/** lateRowNote is a late send's note in the log, which leaves the checks to step 2's line. */
export function lateRowNote(via: SentWith): string {
  return lateness[via].row;
}

/**
 * waitOf is the wait in milliseconds from a send to its arrival, undefined for
 * a gnokey send, whose clock starts at the copy rather than at the call.
 * @param arrivedAt the arrival, in milliseconds since the epoch, as sent.at
 */
export function waitOf(sent: { via: SentWith; at: number }, arrivedAt: number): number | undefined {
  return sent.via === "gnokey" ? undefined : arrivedAt - sent.at;
}

/**
 * arrivedText is a send's line once its notification arrives: the message
 * that arrived, after its wait when the wait is known.
 * @param wait milliseconds from the send to the arrival, undefined when unknown
 * @param body the body the notification delivered
 */
export function arrivedText(wait: number | undefined, body: string): string {
  if (wait === undefined) {
    return `Arrived: "${body}"`;
  }
  return wait < 1000 ? `Arrived within a second: "${body}"` : `Arrived in ${Math.round(wait / 1000)}\u00a0s: "${body}"`;
}

/**
 * bannerCheck is what to check when a push arrived and the OS showed no
 * banner, "" for a live arrival, which the page shows itself.
 */
export function bannerCheck(source: GnotifNotification["source"]): string {
  return source === "push"
    ? "No banner? Check this browser's notification settings in your system settings, and that Focus or Do Not Disturb is off."
    : "";
}

/**
 * sentText is step 2's line once a message goes out: the transaction an
 * Adena send made, or the next step for a copied gnokey command.
 * @param hash the transaction's hash, undefined or "" when Adena gave none
 */
export function sentText(via: SentWith, hash: string | undefined): string {
  if (via === "gnokey") {
    return "Copied. Run it in a terminal, then watch for your message below.";
  }
  return hash === undefined || hash === "" ? "Sent." : `Sent. Transaction ${hash}.`;
}

/**
 * bodyFor is the notification body msg makes: the trigger's body template
 * with msg in place of {msg}, cut as gnotifd delivers it.
 * @param template undefined until the trigger loads, when the body is msg
 */
export function bodyFor(template: string | undefined, msg: string): string {
  return deliveredBody(template === undefined ? msg : template.replaceAll("{msg}", msg));
}

/** waitsFor reports whether a send with body still waits on time for its arrival. */
export function waitsFor(waiting: { body: string; late?: boolean }[], body: string): boolean {
  return waiting.some((sent) => sent.body === body && sent.late !== true);
}
