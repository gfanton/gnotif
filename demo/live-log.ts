// The live log's decisions, apart from the DOM.
import type { GnotifNotification } from "gnotif";

/** WAIT_MS is how long a send waits for its arrival before it reads as late. */
export const WAIT_MS = 60_000;

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
 * clockText is the waiting clock: minutes and seconds since the send, as 1:05.
 * @param ms milliseconds since the send
 */
export function clockText(ms: number): string {
  const s = Math.floor(ms / 1000);
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
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
 * within a second under 1 s.
 * @param ms milliseconds from the send to the arrival
 */
export function waitText(ms: number): string {
  return ms < 1000 ? "Arrived within a second of sending" : `Arrived ${Math.round(ms / 1000)} s after sending`;
}
