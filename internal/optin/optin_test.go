package optin

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gfanton/gnotif/internal/trigger"
)

var stored = map[string]trigger.Trigger{
	"0000001": {ID: "0000001", Param: "next"},
	"0000002": {ID: "0000002"},
}

func lookupStored(_ context.Context, ids []string) (map[string]trigger.Trigger, error) {
	out := map[string]trigger.Trigger{}
	for _, id := range ids {
		if t, ok := stored[id]; ok {
			out[id] = t
		}
	}
	return out, nil
}

func TestCheck(t *testing.T) {
	cases := map[string]struct {
		optins []Optin
		err    string
	}{
		"valid":           {[]Optin{{"0000001", "g1bob"}, {"0000002", ""}}, ""},
		"empty":           {nil, ""},
		"fifty":           {slices.Repeat([]Optin{{"0000002", ""}}, 50), ""},
		"fifty-one":       {slices.Repeat([]Optin{{"0000002", ""}}, 51), "more than 50 opt-ins"},
		"unknown trigger": {[]Optin{{"nope", ""}}, `unknown trigger "nope"`},
		"no param":        {[]Optin{{"0000002", "x"}}, "trigger 0000002 takes no value"},
		"missing value":   {[]Optin{{"0000001", ""}}, "trigger 0000001 needs a value for next"},
		"4096 bytes":      {[]Optin{{"0000001", strings.Repeat("a", 4096)}}, ""},
		"4097 bytes":      {[]Optin{{"0000001", strings.Repeat("a", 4097)}}, "value longer than 4096 bytes"},
		"white space":     {[]Optin{{"0000001", " g1bob"}}, "value has leading or trailing white space"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := Check(context.Background(), tc.optins, lookupStored)
			if tc.err == "" {
				assert.NoError(t, err)
				return
			}
			rule, ok := errors.AsType[Error](err)
			require.True(t, ok, "%v is not a rule error", err)
			assert.Equal(t, tc.err, rule.Error())
		})
	}
}

func TestCheckLooksUpEachNamedTriggerOnce(t *testing.T) {
	var asked [][]string
	lookup := func(ctx context.Context, ids []string) (map[string]trigger.Trigger, error) {
		asked = append(asked, ids)
		return lookupStored(ctx, ids)
	}
	require.NoError(t, Check(context.Background(), []Optin{{"0000002", ""}, {"0000001", "g1bob"}, {"0000002", ""}}, lookup))
	assert.Equal(t, [][]string{{"0000001", "0000002"}}, asked)

	asked = nil
	require.Error(t, Check(context.Background(), slices.Repeat([]Optin{{"0000002", ""}}, 51), lookup))
	assert.Empty(t, asked, "a set over the limit is refused without a lookup")
}

func TestCheckReturnsLookupFailures(t *testing.T) {
	boom := errors.New("database is locked")
	lookup := func(context.Context, []string) (map[string]trigger.Trigger, error) { return nil, boom }
	err := Check(context.Background(), []Optin{{"0000002", ""}}, lookup)
	assert.ErrorIs(t, err, boom)
	_, isRule := errors.AsType[Error](err)
	assert.False(t, isRule)
}
