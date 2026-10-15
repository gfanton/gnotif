// The live log's decisions, apart from the DOM.

/** WAIT_MS is how long a send waits for its arrival before it reads as late. */
export const WAIT_MS = 60_000;

/**
 * answeredSend returns the index of the oldest send in waiting whose
 * notification has body, or -1 when none waits for it.
 * @param {{ body: string }[]} waiting oldest first
 * @param {string} body the arrival's notification body
 * @returns {number}
 */
export function answeredSend(waiting, body) {
  return waiting.findIndex((sent) => sent.body === body);
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
 * waitText says how long an arrival took after its send.
 * @param {number} ms milliseconds from the send to the arrival
 * @returns {string}
 */
export function waitText(ms) {
  return ms < 1000
    ? "Arrived within a second of sending"
    : `Arrived ${Math.round(ms / 1000)} s after sending`;
}
