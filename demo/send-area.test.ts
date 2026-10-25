import test from "node:test";
import assert from "node:assert/strict";

import { sendArea } from "./send-area.ts";

const W = "g1zrjyzatudmadgqu39d5wfamhdvf0adwlxkz25d";

test("connected to Adena, Send with Adena leads", () => {
  assert.deepEqual(sendArea({ adena: "found", wallet: W, address: W }), { adenaSend: "enabled", gnokeyLeads: false });
});

test("with Adena found but not connected, Send with Adena waits for Connect", () => {
  assert.deepEqual(sendArea({ adena: "found", wallet: "", address: "" }), {
    adenaSend: "disabled",
    gnokeyLeads: false,
  });
});

test("a typed address sends with gnokey, even with Adena found", () => {
  assert.deepEqual(sendArea({ adena: "found", wallet: "", address: W }), { adenaSend: "hidden", gnokeyLeads: true });
});

test("without Adena, gnokey is the only way to send", () => {
  for (const adena of ["absent", "pending"] as const) {
    assert.deepEqual(sendArea({ adena, wallet: "", address: "" }), { adenaSend: "hidden", gnokeyLeads: true }, adena);
  }
});
