// Package gnotif is a registry of notification triggers. Each trigger names
// a realm and an event type to watch, and the text of the notification that
// gnotif servers send when a matching event lands on chain. A later registry
// version follows the rules in
// https://gnotif.xyz/docs/how-it-works/#rules-for-a-new-registry-version.
package gnotif

import "gno.land/p/dev/gnotif/trigger/v0"

// Subpath is the subpath of a realm's gnotif identity: me := cur.Sub(gnotif.Subpath).
const Subpath = "gnotif"

// Trigger is an alias, so a dapp imports only the registry.
type Trigger = trigger.Trigger
