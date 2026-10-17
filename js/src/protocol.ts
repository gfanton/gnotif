// Each parse function gives undefined for data that does not match its schema.

import * as v from "valibot";

import type { GnotifNotification, Trigger } from "./gnotif.ts";

export type NotificationFields = Omit<GnotifNotification, "source">;

const TriggerSchema: v.GenericSchema<Trigger> = v.object({
  id: v.string(),
  target: v.string(),
  event: v.string(),
  filter: v.string(),
  param: v.string(),
  title: v.string(),
  body: v.string(),
  link: v.string(),
  declarer: v.string(),
  verified: v.boolean(),
});

const TriggersSchema = v.array(TriggerSchema);

const VapidSchema = v.object({ publicKey: v.string() });

const ErrorBodySchema = v.object({ error: v.optional(v.string()) });

const notificationEntries = {
  id: v.string(),
  trigger: v.string(),
  value: v.string(),
  title: v.string(),
  body: v.string(),
  link: v.string(),
  height: v.number(),
  tx: v.string(),
  time: v.string(),
  attrs: v.array(v.object({ key: v.string(), value: v.string() })),
};

const NotificationSchema: v.GenericSchema<NotificationFields> = v.object(notificationEntries);

const FrameSchema = v.variant("type", [
  v.object({ type: v.literal("pong") }),
  v.object({ type: v.literal("optins") }),
  v.object({ type: v.literal("error"), error: v.fallback(v.optional(v.string()), undefined) }),
  v.object({ type: v.literal("notification"), ...notificationEntries }),
]);

export type Frame = v.InferOutput<typeof FrameSchema>;

const RelaySchema = v.object({ type: v.literal("gnotif.notification"), notification: NotificationSchema });

function parsed<T>(schema: v.GenericSchema<unknown, T>, data: unknown): T | undefined {
  const result = v.safeParse(schema, data);
  return result.success ? result.output : undefined;
}

export function parseTriggers(data: unknown): Trigger[] | undefined {
  return parsed(TriggersSchema, data);
}

export function parseVapidKey(data: unknown): string | undefined {
  return parsed(VapidSchema, data)?.publicKey;
}

export function parseErrorMessage(data: unknown): string | undefined {
  return parsed(ErrorBodySchema, data)?.error;
}

export function parseFrame(text: string): Frame | undefined {
  let data: unknown;
  try {
    data = JSON.parse(text);
  } catch {
    return undefined;
  }
  return parsed(FrameSchema, data);
}

export function parseRelay(data: unknown): NotificationFields | undefined {
  return parsed(RelaySchema, data)?.notification;
}
