package query

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/shared/apperr"
)

// The input's defaults when the repo sets none.
const (
	defaultTimezone     = "Etc/UTC"
	defaultWeekStartDay = "monday"
)

// withRepoDefaults is in with the repo's defaults filled in: an AQL query's
// time zone and week start day, the repo's or else anfra's. It refuses a query
// that changes a default the repo locks, and every query when the repo's
// defaults can't be used (docs/designs/query-settings.md). The cache's default
// duration is applied where the cache is looked up.
func (in QueryRequestParams) withRepoDefaults(cc command.CommandContext) (QueryRequestParams, error) {
	s := cc.QueryDefaults
	if err := cmp.Or(s.Err, s.Check()); err != nil {
		return in, apperr.Encapsulate(err, errcode.RepoSettingsInvalid, sentence(err.Error()))
	}
	if err := in.checkLockedDuration(s); err != nil {
		return in, err
	}
	if in.Lang == "sql" {
		return in, nil // compiled by no one: neither applies
	}
	tz, err := lockedOrDefault(s, "input.timezone", in.timezoneName(), in.Input.Timezone, cmp.Or(s.Timezone, defaultTimezone))
	if err != nil {
		return in, err
	}
	day, err := lockedOrDefault(s, "input.weekStartDay", "input.weekStartDay", in.Input.WeekStartDay, cmp.Or(s.WeekStartDay, defaultWeekStartDay))
	if err != nil {
		return in, err
	}
	in.Input.Timezone, in.Input.WeekStartDay = tz, day
	return in, nil
}

// lockedOrDefault is the value an input runs with: sent, the repo's default
// when it sends none, and refused when it sends another than a locked default.
// setting is the default's path in the repo's settings, arg the input's name.
func lockedOrDefault(s command.QueryDefaults, setting, arg, sent, def string) (string, error) {
	if sent == "" {
		return def, nil
	}
	if sent != def && slices.Contains(s.Locked, setting) {
		return "", lockedArg(arg, sent, def)
	}
	return sent, nil
}

// checkLockedDuration refuses a cache.duration other than the repo's default
// when the repo locks it.
func (in QueryRequestParams) checkLockedDuration(s command.QueryDefaults) error {
	sent, set := in.Cache.duration()
	if set && sent != s.CacheDuration && slices.Contains(s.Locked, "cache.duration") {
		return lockedArg("cache.duration", in.Cache.Duration, command.FormatDuration(s.CacheDuration))
	}
	return nil
}

func lockedArg(arg, sent, locked string) error {
	return command.InvalidArg(arg, "invalid", fmt.Sprintf("%s is %s, but the repo locks it at %s (query.locked_defaults in the repo's settings). Leave it out, or send %s.", arg, sent, locked, locked))
}

// sentence is msg ending as a sentence does, with a full stop unless it ends
// with one, or a question mark.
func sentence(msg string) string {
	if strings.HasSuffix(msg, ".") || strings.HasSuffix(msg, "?") {
		return msg
	}
	return msg + "."
}
