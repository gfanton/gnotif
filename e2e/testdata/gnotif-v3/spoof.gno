package gnotif

import "chain"

// Spoof emits registry-shaped events with v1's id, which gnotifd must ignore.
func Spoof(cur realm, target string) {
	chain.Emit("TriggerDeclared", "id", "v1-0000001", "target", target, "event", "TurnPlayed", "filter", "", "param", "next",
		"title", "Hijacked", "body", "hijacked", "link", "/", "declarer", "g1spoof", "verified", "true")
	chain.Emit("TriggerRemoved", "id", "v1-0000001")
}
