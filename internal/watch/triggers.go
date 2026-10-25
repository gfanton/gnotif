package watch

import (
	"cmp"
	"context"
	"maps"
	"slices"

	"github.com/gfanton/gnotif/internal/store"
	"github.com/gfanton/gnotif/internal/trigger"
)

// Triggers is an immutable snapshot of every declared trigger. Any number of
// goroutines may read one; a change builds the next snapshot with edit.
type Triggers struct {
	byID     map[string]trigger.Trigger
	byTarget map[string][]trigger.Trigger // by registry version, then id
}

func newTriggers(byID map[string]trigger.Trigger) *Triggers {
	byTarget := make(map[string][]trigger.Trigger)
	for _, tr := range byID {
		byTarget[tr.Target] = append(byTarget[tr.Target], tr)
	}
	for _, list := range byTarget {
		slices.SortFunc(list, func(a, b trigger.Trigger) int {
			return cmp.Or(cmp.Compare(idVersion(a.ID), idVersion(b.ID)), cmp.Compare(a.ID, b.ID))
		})
	}
	return &Triggers{byID: byID, byTarget: byTarget}
}

// NewTriggers returns a snapshot of ts. Of two triggers with one id, the
// later wins.
func NewTriggers(ts []trigger.Trigger) *Triggers {
	byID := make(map[string]trigger.Trigger, len(ts))
	for _, tr := range ts {
		byID[tr.ID] = tr
	}
	return newTriggers(byID)
}

// Matching returns the triggers on target for event, by registry version,
// then id.
func (t *Triggers) Matching(target, event string) []trigger.Trigger {
	var out []trigger.Trigger
	for _, tr := range t.byTarget[target] {
		if tr.Event == event {
			out = append(out, tr)
		}
	}
	return out
}

// TargetTriggers returns the triggers of one realm by registry version, then
// id, at most store.MaxPerTarget from each version. The error is always nil.
func (t *Triggers) TargetTriggers(_ context.Context, target string) ([]trigger.Trigger, error) {
	var out []trigger.Trigger
	version, n := -1, 0
	for _, tr := range t.byTarget[target] {
		if v := idVersion(tr.ID); v != version {
			version, n = v, 0
		}
		if n < store.MaxPerTarget {
			out = append(out, tr)
			n++
		}
	}
	return out, nil
}

// TriggersByID returns the triggers among ids, keyed by id. Unknown ids are
// absent. The error is always nil; the signature is optin.Lookup's.
func (t *Triggers) TriggersByID(_ context.Context, ids []string) (map[string]trigger.Trigger, error) {
	out := make(map[string]trigger.Trigger, len(ids))
	for _, id := range ids {
		if tr, ok := t.byID[id]; ok {
			out[id] = tr
		}
	}
	return out, nil
}

// triggerEdit builds the snapshot that follows a Triggers. It is not safe
// for concurrent use.
type triggerEdit struct {
	byID map[string]trigger.Trigger
}

// edit starts a change on a copy; t stays as it is.
func (t *Triggers) edit() *triggerEdit {
	return &triggerEdit{byID: maps.Clone(t.byID)}
}

// put adds tr, or replaces the trigger with its id.
func (e *triggerEdit) put(tr trigger.Trigger) { e.byID[tr.ID] = tr }

func (e *triggerEdit) delete(id string) { delete(e.byID, id) }

// done returns the edited snapshot. The edit must not be used afterwards.
func (e *triggerEdit) done() *Triggers { return newTriggers(e.byID) }
