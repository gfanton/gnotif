// Package optin holds the rules an opt-in set follows, for push
// subscriptions and live connections alike.
package optin

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/gfanton/gnotif/internal/trigger"
)

// Limits of an opt-in set.
const (
	Max      = 50
	MaxValue = 4096
)

// Optin asks for one trigger's notifications. Value is the trigger's param
// value, empty for a trigger without a param.
type Optin struct {
	Trigger string `json:"trigger"`
	Value   string `json:"value"`
}

// Error is a rule an opt-in set breaks. Its text is the message the API
// answers with.
type Error string

func (e Error) Error() string { return string(e) }

// Lookup returns the stored triggers among ids, keyed by id.
type Lookup func(ctx context.Context, ids []string) (map[string]trigger.Trigger, error)

// Check reports the first rule optins breaks, as an Error. It looks up only
// the triggers the set names, and none for a set over Max; an error from
// lookup comes back as it is.
func Check(ctx context.Context, optins []Optin, lookup Lookup) error {
	if len(optins) > Max {
		return Error(fmt.Sprintf("more than %d opt-ins", Max))
	}
	ids := make([]string, 0, len(optins))
	for _, o := range optins {
		ids = append(ids, o.Trigger)
	}
	slices.Sort(ids)
	byID, err := lookup(ctx, slices.Compact(ids))
	if err != nil {
		return err
	}
	for _, o := range optins {
		t, ok := byID[o.Trigger]
		switch {
		case !ok:
			return Error(fmt.Sprintf("unknown trigger %q", o.Trigger))
		case t.Param == "" && o.Value != "":
			return Error(fmt.Sprintf("trigger %s takes no value", t.ID))
		case t.Param != "" && o.Value == "":
			return Error(fmt.Sprintf("trigger %s needs a value for %s", t.ID, t.Param))
		case len(o.Value) > MaxValue:
			return Error(fmt.Sprintf("value longer than %d bytes", MaxValue))
		case o.Value != strings.TrimSpace(o.Value):
			// A value with surrounding white space never equals an event
			// attribute.
			return Error("value has leading or trailing white space")
		}
	}
	return nil
}
