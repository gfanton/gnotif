import { test } from "node:test";
import assert from "node:assert/strict";
import { isAddress } from "./address.mjs";

const valid = [
  "g1zrjyzatudmadgqu39d5wfamhdvf0adwlxkz25d",
  "g13rqvks880v0hmlfepcj0kkudq0js38d3pc5k6a",
  "g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5",
];
const a = valid[0];

test("accepts well-formed addresses", () => {
  for (const s of valid) {
    assert.equal(isAddress(s), true, s);
  }
});

test("rejects malformed addresses", () => {
  const cases = {
    "trailing dot": `${a}.`,
    "leading space": ` ${a}`,
    "trailing space": `${a} `,
    uppercase: a.toUpperCase(),
    "bad checksum": `${a.slice(0, -1)}q`,
    "39 characters": a.slice(0, -1),
    "41 characters": `${a}q`,
    "b in data": `g1b${a.slice(3)}`,
    "i in data": `g1i${a.slice(3)}`,
    "o in data": `g1o${a.slice(3)}`,
    "1 in data": `g11${a.slice(3)}`,
    "other prefix": "cosmos1zrjyzatudmadgqu39d5wfamhdvf0adwlxkz25d",
    empty: "",
  };
  for (const [name, s] of Object.entries(cases)) {
    assert.equal(isAddress(s), false, name);
  }
});
