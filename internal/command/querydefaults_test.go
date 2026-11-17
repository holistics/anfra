package command

import (
	"strings"
	"testing"
	"time"
)

// Check takes anfra's defaults, and a host's that a query can run with; it
// refuses the rest, naming the setting as the repo's settings do.
func TestQueryDefaultsCheck(t *testing.T) {
	for _, ok := range []QueryDefaults{
		{},
		{Timezone: "Asia/Ho_Chi_Minh", WeekStartDay: "sunday", CacheDuration: time.Hour, Locked: []string{"input.timezone", "input.weekStartDay", "cache.duration"}},
		{Locked: []string{"input.timezone"}}, // locked at anfra's default, Etc/UTC
	} {
		if err := ok.Check(); err != nil {
			t.Errorf("%+v: %v", ok, err)
		}
	}
	for name, tc := range map[string]struct {
		d    QueryDefaults
		want string
	}{
		"a time zone":       {QueryDefaults{Timezone: "Mars/Olympus"}, `query.default.input.timezone is "Mars/Olympus", which isn't an IANA time zone`},
		"the local zone":    {QueryDefaults{Timezone: "Local"}, "isn't an IANA time zone"},
		"a capitalized day": {QueryDefaults{WeekStartDay: "Monday"}, `query.default.input.weekStartDay is "Monday". Did you mean monday?`},
		"not a day":         {QueryDefaults{WeekStartDay: "weekend"}, "Use one of monday, tuesday, wednesday, thursday, friday, saturday, and sunday"},
		"a negative cache":  {QueryDefaults{CacheDuration: -time.Minute}, "query.default.cache.duration is -1m, which is negative"},
		"an object": {QueryDefaults{Locked: []string{"input"}},
			"query.locked_defaults names input, which has fields that can't have a default. Lock its fields one by one, such as input.timezone and input.weekStartDay"},
		"not a default": {QueryDefaults{Locked: []string{"input.pagination"}},
			"query.locked_defaults names input.pagination, which isn't a default. The defaults are input.timezone, input.weekStartDay, and cache.duration"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := tc.d.Check(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v\nwant one containing %q", err, tc.want)
			}
		})
	}
}
