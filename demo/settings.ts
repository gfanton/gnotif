// What the demo remembers in this browser. The server cannot read opt-ins
// back, so the page keeps the last ones it set.

import * as v from "valibot";

const KEY = "gnotif-demo";

const OptinSchema = v.object({ trigger: v.string(), value: v.string() });

const OptinsSchema = v.pipe(
  v.array(v.unknown()),
  v.transform((items) =>
    items.flatMap((item) => {
      const result = v.safeParse(OptinSchema, item);
      return result.success ? [result.output] : [];
    }),
  ),
);

const SettingsSchema = v.object({
  address: v.fallback(v.string(), ""),
  optins: v.fallback(OptinsSchema, []),
  /** whether the visitor turned live updates on */
  live: v.fallback(v.boolean(), false),
  /** whether the visitor turned push notifications on */
  push: v.fallback(v.boolean(), true),
});

export type Settings = v.InferOutput<typeof SettingsSchema>;

/**
 * Reads the stored settings. A record without `push` reads as push on.
 * @returns null when nothing usable is stored
 */
export function load(storage: Pick<Storage, "getItem">): Settings | null {
  let data: unknown;
  try {
    data = JSON.parse(storage.getItem(KEY) ?? "null");
  } catch {
    return null;
  }
  const result = v.safeParse(SettingsSchema, data);
  return result.success ? result.output : null;
}

export function save(storage: Pick<Storage, "setItem">, settings: Settings): void {
  storage.setItem(KEY, JSON.stringify(settings));
}
