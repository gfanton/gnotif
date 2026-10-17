package watch

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gfanton/gnotif/internal/store"
	"github.com/gfanton/gnotif/internal/trigger"
)

func triggerAt(id, target, event string) trigger.Trigger {
	return trigger.Trigger{ID: id, Target: target, Event: event, Title: "t", Body: "b", Link: "/", Declarer: "g1alice"}
}

func triggerIDs(ts []trigger.Trigger) []string {
	if len(ts) == 0 {
		return nil
	}
	ids := make([]string, len(ts))
	for i, tr := range ts {
		ids[i] = tr.ID
	}
	return ids
}

func TestTriggersTargetTriggers(t *testing.T) {
	ctx := context.Background()
	ts := []trigger.Trigger{
		triggerAt("b", "gno.land/r/a", "E"),
		triggerAt("a", "gno.land/r/a", "E"),
		triggerAt("c", "gno.land/r/other", "E"),
	}
	for i := range store.MaxPerTarget + 1 {
		ts = append(ts, triggerAt(fmt.Sprintf("many%03d", i), "gno.land/r/many", "E"))
	}
	s := NewTriggers(ts)

	got, err := s.TargetTriggers(ctx, "gno.land/r/a")
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, triggerIDs(got))

	got, err = s.TargetTriggers(ctx, "gno.land/r/none")
	require.NoError(t, err)
	assert.Empty(t, got)

	got, err = s.TargetTriggers(ctx, "gno.land/r/many")
	require.NoError(t, err)
	require.Len(t, got, store.MaxPerTarget)
	assert.Equal(t, "many000", got[0].ID)
	assert.Equal(t, fmt.Sprintf("many%03d", store.MaxPerTarget-1), got[store.MaxPerTarget-1].ID)
}

func TestTriggersTriggersByID(t *testing.T) {
	ctx := context.Background()
	s := NewTriggers([]trigger.Trigger{
		triggerAt("a", "gno.land/r/a", "E"),
		triggerAt("b", "gno.land/r/a", "E"),
		triggerAt("c", "gno.land/r/a", "E"),
	})

	cases := map[string]struct {
		ids  []string
		want []string
	}{
		"subset":         {[]string{"a", "c"}, []string{"a", "c"}},
		"unknown absent": {[]string{"a", "nope"}, []string{"a"}},
		"duplicates":     {[]string{"b", "b", "b"}, []string{"b"}},
		"only unknown":   {[]string{"nope"}, nil},
		"empty":          {nil, nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := s.TriggersByID(ctx, tc.ids)
			require.NoError(t, err)
			keys := make([]string, 0, len(got))
			for id, tr := range got {
				assert.Equal(t, id, tr.ID)
				keys = append(keys, id)
			}
			assert.ElementsMatch(t, tc.want, keys)
		})
	}
}

func TestTriggersMatching(t *testing.T) {
	s := NewTriggers([]trigger.Trigger{
		triggerAt("b", "gno.land/r/a", "E"),
		triggerAt("a", "gno.land/r/a", "E"),
		triggerAt("c", "gno.land/r/a", "Other"),
		triggerAt("d", "gno.land/r/b", "E"),
	})

	cases := map[string]struct {
		target, event string
		want          []string
	}{
		"both match":   {"gno.land/r/a", "E", []string{"a", "b"}},
		"other event":  {"gno.land/r/a", "Other", []string{"c"}},
		"other target": {"gno.land/r/b", "E", []string{"d"}},
		"nothing":      {"gno.land/r/b", "Other", nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, triggerIDs(s.Matching(tc.target, tc.event)))
		})
	}
}

func TestTriggersPutReplacesSameID(t *testing.T) {
	e := NewTriggers([]trigger.Trigger{triggerAt("a", "gno.land/r/a", "E")}).edit()
	moved := triggerAt("a", "gno.land/r/b", "F")
	e.put(moved)
	n := e.done()

	assert.Empty(t, n.Matching("gno.land/r/a", "E"))
	assert.Equal(t, []trigger.Trigger{moved}, n.Matching("gno.land/r/b", "F"))
}

func TestEditLeavesOriginalUnchanged(t *testing.T) {
	ctx := context.Background()
	s := NewTriggers([]trigger.Trigger{triggerAt("T1", "gno.land/r/a", "E")})

	e := s.edit()
	e.put(triggerAt("T2", "gno.land/r/a", "E"))
	e.delete("T1")
	n := e.done()

	got, err := s.TriggersByID(ctx, []string{"T1", "T2"})
	require.NoError(t, err)
	assert.Contains(t, got, "T1")
	assert.NotContains(t, got, "T2")
	assert.Equal(t, []string{"T1"}, triggerIDs(s.Matching("gno.land/r/a", "E")))

	got, err = n.TriggersByID(ctx, []string{"T1", "T2"})
	require.NoError(t, err)
	assert.NotContains(t, got, "T1")
	assert.Contains(t, got, "T2")
	assert.Equal(t, []string{"T2"}, triggerIDs(n.Matching("gno.land/r/a", "E")))
}

func TestTriggersConcurrentReads(t *testing.T) {
	ctx := context.Background()
	var cur atomic.Pointer[Triggers]
	cur.Store(NewTriggers([]trigger.Trigger{triggerAt("T0", "gno.land/r/a", "E")}))

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 1000 {
				got, err := cur.Load().TriggersByID(ctx, []string{"T0", "T1"})
				assert.NoError(t, err)
				assert.Contains(t, got, "T0")
			}
		})
	}
	wg.Go(func() {
		for i := range 1000 {
			e := cur.Load().edit()
			e.put(triggerAt(fmt.Sprintf("W%d", i), "gno.land/r/a", "E"))
			cur.Store(e.done())
		}
	})
	wg.Wait()

	got, err := cur.Load().TriggersByID(ctx, []string{"T0", "W999"})
	require.NoError(t, err)
	assert.Len(t, got, 2)
}
