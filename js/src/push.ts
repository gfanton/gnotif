// The Web Push side that the client and the service worker both read. Each
// parse function gives undefined, a fallback or null for data that does not
// match its schema.

import * as v from "valibot";

const VapidSchema = v.object({ publicKey: v.string() });

const PayloadSchema = v.object({ title: v.pipe(v.string(), v.nonEmpty()), body: v.string(), link: v.string() });

const ClickDataSchema = v.object({ link: v.string() });

const RelayableSchema = v.looseObject({ id: v.string() });

export function parsed<T>(schema: v.GenericSchema<unknown, T>, data: unknown): T | undefined {
  const result = v.safeParse(schema, data);
  return result.success ? result.output : undefined;
}

export function parseJson(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
}

export function parseVapidKey(data: unknown): string | undefined {
  return parsed(VapidSchema, data)?.publicKey;
}

export function parsePayload(text: string): { title: string; body: string; link: string } {
  return parsed(PayloadSchema, parseJson(text)) ?? { title: "New activity", body: "", link: "./" };
}

export function relayable(text: string): ({ id: string } & Record<string, unknown>) | null {
  return parsed(RelayableSchema, parseJson(text)) ?? null;
}

export function parseClickLink(data: unknown): string {
  return parsed(ClickDataSchema, data)?.link ?? "./";
}
