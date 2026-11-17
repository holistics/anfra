package command

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/holistics/anfra/internal/didyoumean"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
)

// QueryDefaults are what a query gets when it sets none, and which of them it
// can't change: a repo's query.default and query.locked_defaults
// (docs/designs/query-settings.md). A host sets them from wherever it keeps a
// repo's settings, and checks them with Check.
type QueryDefaults struct {
	// Timezone is input.timezone's default, an IANA zone: empty, Etc/UTC.
	Timezone string
	// WeekStartDay is input.weekStartDay's default, one of
	// anfranode.WeekStartDays: empty, monday.
	WeekStartDay string
	// CacheDuration is cache.duration's default: zero, a query that sets none
	// runs uncached.
	CacheDuration time.Duration
	// Locked are the defaults a query can't change, by their path in the
	// repo's settings: input.timezone, input.weekStartDay, cache.duration.
	// A query may leave a locked one out, or send its value.
	Locked []string
	// Err is why the repo's defaults can't be used. Every query is refused
	// with it, rather than run with defaults the repo may not mean.
	Err error
}

// LockablePaths are the defaults Locked may name, as the repo's settings
// name them.
var LockablePaths = []string{"input.timezone", "input.weekStartDay", "cache.duration"}

// Check refuses defaults no query can run with, each error naming the setting
// as the repo's settings do (query.default.input.timezone): a time zone or a
// week start day anfra doesn't take, a negative cache duration, or a lock on
// something that isn't a default. A query refuses with it too, so a host that
// skips it fails on the query instead.
func (d QueryDefaults) Check() error {
	if s := d.Timezone; s != "" {
		if _, err := time.LoadLocation(s); err != nil || s == "Local" {
			return fmt.Errorf("query.default.input.timezone is %q, which isn't an IANA time zone. Use one such as Asia/Ho_Chi_Minh or Etc/UTC", s)
		}
	}
	if s := d.WeekStartDay; s != "" && !slices.Contains(anfranode.WeekStartDays, s) {
		if near := didyoumean.Find(s, anfranode.WeekStartDays); near != "" {
			return fmt.Errorf("query.default.input.weekStartDay is %q. Did you mean %s?", s, near)
		}
		return fmt.Errorf("query.default.input.weekStartDay is %q. Use one of %s", s, prose(anfranode.WeekStartDays))
	}
	if d.CacheDuration < 0 {
		return fmt.Errorf("query.default.cache.duration is %s, which is negative. Use 0s or more", FormatDuration(d.CacheDuration))
	}
	for _, path := range d.Locked {
		if slices.Contains(LockablePaths, path) {
			continue
		}
		var under []string
		for _, p := range LockablePaths {
			if strings.HasPrefix(p, path+".") {
				under = append(under, p)
			}
		}
		if len(under) > 0 {
			return fmt.Errorf("query.locked_defaults names %s, which has fields that can't have a default. Lock its fields one by one, such as %s", path, prose(under))
		}
		return fmt.Errorf("query.locked_defaults names %s, which isn't a default. The defaults are %s", path, prose(LockablePaths))
	}
	return nil
}

// FormatDuration is d as a person writes it: 10m and 1h, not 10m0s and
// 1h0m0s.
func FormatDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}

// prose is names as a sentence lists them: "a", "a and b", or "a, b, and c".
func prose(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	case 2:
		return names[0] + " and " + names[1]
	}
	return strings.Join(names[:len(names)-1], ", ") + ", and " + names[len(names)-1]
}
