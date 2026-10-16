// The live log's decisions, apart from the DOM.

/** WAIT_MS is how long a send waits for its arrival before it reads as late. */
export const WAIT_MS = 60_000;

/**
 * answeredSend returns the index of the oldest send in waiting whose
 * notification has body, taking one still on time before a late one, or -1
 * when none waits for it.
 * @param {{ body: string, late?: boolean }[]} waiting oldest first
 * @param {string} body the arrival's notification body
 * @returns {number}
 */
export function answeredSend(waiting, body) {
  const onTime = waiting.findIndex((sent) => sent.body === body && sent.late !== true);
  return onTime !== -1 ? onTime : waiting.findIndex((sent) => sent.body === body);
}

/** MAX_BODY is gnotifd's limit on a notification body, in UTF-8 bytes. */
export const MAX_BODY = 255;

/**
 * deliveredBody is body as gnotifd delivers it: cut to MAX_BODY bytes of
 * UTF-8, at the start of a character.
 * @param {string} body
 * @returns {string}
 */
export function deliveredBody(body) {
  const bytes = new TextEncoder().encode(body);
  if (bytes.length <= MAX_BODY) {
    return body;
  }
  let cut = MAX_BODY;
  while (cut > 0 && (bytes[cut] & 0xc0) === 0x80) {
    cut--;
  }
  return new TextDecoder().decode(bytes.subarray(0, cut));
}

/**
 * clockText is the waiting clock: minutes and seconds since the send, as 1:05.
 * @param {number} ms milliseconds since the send
 * @returns {string}
 */
export function clockText(ms) {
  const s = Math.floor(ms / 1000);
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

/**
 * blockSummary is an arrival's line in the log: its block, and how long it
 * took after its send when one waited for it.
 * @param {number} height
 * @param {number | undefined} wait milliseconds from the send to the arrival
 * @returns {string}
 */
export function blockSummary(height, wait) {
  return wait === undefined ? `Block ${height}` : `Block ${height} · ${waitText(wait)}`;
}

/**
 * details lists what an arrival carries besides its block, as label and value
 * pairs: the block's time as format writes it, the transaction, the trigger
 * with its opted-in value, the event's attributes and the link. A time that
 * does not parse is left out.
 * @param {import("./gnotif.js").LiveNotification} n
 * @param {(at: Date) => string} format
 * @returns {[string, string][]}
 */
export function details(n, format) {
  const at = new Date(n.time);
  /** @type {[string, string][]} */
  const rows = Number.isNaN(at.getTime()) ? [] : [["Block time", format(at)]];
  rows.push(["Transaction", n.tx], ["Trigger", n.value === "" ? n.trigger : `${n.trigger} for ${n.value}`]);
  for (const { key, value } of n.attrs) {
    rows.push([key, value]);
  }
  rows.push(["Link", n.link]);
  return rows;
}

/**
 * waitText says how long an arrival took after its send.
 * @param {number} ms milliseconds from the send to the arrival
 * @returns {string}
 */
export function waitText(ms) {
  return ms < 1000
    ? "Arrived within a second of sending"
    : `Arrived ${Math.round(ms / 1000)} s after sending`;
}
