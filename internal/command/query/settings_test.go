package query

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/shared/apperr"
)

// input.weekStartDay's schema lists the days anfra-node takes, as a struct tag
// can't name them.
func TestWeekStartDayEnum(t *testing.T) {
	f, _ := reflect.TypeFor[QueryInput]().FieldByName("WeekStartDay")
	if got := strings.Split(f.Tag.Get("enum"), ","); !slices.Equal(got, anfranode.WeekStartDays) {
		t.Errorf("enum %v, want anfranode.WeekStartDays %v", got, anfranode.WeekStartDays)
	}
}

// An AQL query gets the repo's defaults, or anfra's, for what it leaves out,
// and keeps what it sends; a SQL query gets neither.
func TestWithRepoSettingsDefaults(t *testing.T) {
	aql := func(in QueryInput) QueryRequestParams {
		return QueryRequestParams{QueryDefinition: QueryDefinition{Dataset: "d", Query: "q"}, Input: in}
	}
	for name, tc := range map[string]struct {
		settings      command.QueryDefaults
		in            QueryInput
		wantTZ, wantD string
	}{
		"anfra's":             {command.QueryDefaults{}, QueryInput{}, "Etc/UTC", "monday"},
		"the repo's":          {command.QueryDefaults{Timezone: "Asia/Ho_Chi_Minh", WeekStartDay: "sunday"}, QueryInput{}, "Asia/Ho_Chi_Minh", "sunday"},
		"the query's":         {command.QueryDefaults{Timezone: "Asia/Ho_Chi_Minh", WeekStartDay: "sunday"}, QueryInput{Timezone: "Europe/London", WeekStartDay: "tuesday"}, "Europe/London", "tuesday"},
		"a lock's own value":  {command.QueryDefaults{WeekStartDay: "sunday", Locked: []string{"input.weekStartDay"}}, QueryInput{WeekStartDay: "sunday"}, "Etc/UTC", "sunday"},
		"another, not locked": {command.QueryDefaults{WeekStartDay: "sunday", Locked: []string{"input.timezone"}}, QueryInput{WeekStartDay: "monday"}, "Etc/UTC", "monday"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := aql(tc.in).withRepoDefaults(command.CommandContext{QueryDefaults: tc.settings})
			if err != nil || got.Input.Timezone != tc.wantTZ || got.Input.WeekStartDay != tc.wantD {
				t.Errorf("got %q, %q, %v; want %q, %q", got.Input.Timezone, got.Input.WeekStartDay, err, tc.wantTZ, tc.wantD)
			}
			if r := got.run(); r.Timezone != tc.wantTZ || r.WeekStartDay != tc.wantD {
				t.Errorf("run %+v, want what the input holds", r)
			}
		})
	}

	sql := QueryRequestParams{QueryDefinition: QueryDefinition{Lang: "sql", DataSource: "w", Query: "select 1"}}
	got, err := sql.withRepoDefaults(command.CommandContext{QueryDefaults: command.QueryDefaults{Timezone: "Asia/Ho_Chi_Minh"}})
	if err != nil || !got.Input.IsZero() {
		t.Errorf("SQL: input %+v, %v; want none filled in", got.Input, err)
	}
}

// A query that changes a locked default is refused on the input it sent, with
// the value to send; a repo whose settings can't be used refuses every query.
func TestWithRepoSettingsRefuses(t *testing.T) {
	locked := command.QueryDefaults{Timezone: "Asia/Ho_Chi_Minh", WeekStartDay: "monday", CacheDuration: 10 * time.Minute,
		Locked: []string{"input.timezone", "input.weekStartDay", "cache.duration"}}
	cc := command.CommandContext{QueryDefaults: locked}
	in := func(f func(*QueryRequestParams)) QueryRequestParams {
		p := QueryRequestParams{QueryDefinition: QueryDefinition{Dataset: "d", Query: "q"}}
		f(&p)
		return p.normalize()
	}
	for name, tc := range map[string]struct {
		in        QueryRequestParams
		arg, want string
	}{
		"the week start day": {in(func(p *QueryRequestParams) { p.Input.WeekStartDay = "sunday" }), "input.weekStartDay",
			"input.weekStartDay is sunday, but the repo locks it at monday (query.locked_defaults in the repo's settings). Leave it out, or send monday."},
		"the time zone":            {in(func(p *QueryRequestParams) { p.Input.Timezone = "Etc/UTC" }), "input.timezone", "the repo locks it at Asia/Ho_Chi_Minh"},
		"the deprecated time zone": {in(func(p *QueryRequestParams) { p.Timezone = "Etc/UTC" }), "timezone", "timezone is Etc/UTC, but the repo locks it at Asia/Ho_Chi_Minh"},
		"the cache duration": {in(func(p *QueryRequestParams) { p.Cache.Duration = "1h" }), "cache.duration",
			"cache.duration is 1h, but the repo locks it at 10m"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := tc.in.withRepoDefaults(cc)
			v, _ := apperr.DetailsOf(err, apperr.ValidationFailed)
			if len(v.Violations) != 1 || v.Violations[0].Field != tc.arg || !strings.Contains(v.Violations[0].Message, tc.want) {
				t.Errorf("err = %v, want a violation on %s containing %q", err, tc.arg, tc.want)
			}
		})
	}

	same := in(func(p *QueryRequestParams) { p.Cache.Duration = "600s"; p.Input.WeekStartDay = "monday" })
	if _, err := same.withRepoDefaults(cc); err != nil {
		t.Errorf("the locked values sent back: %v, want accepted", err)
	}

	broken := errors.New(".anfra/config.yml line 3: query.cache.enabeld isn't a setting. Did you mean enabled?")
	_, err := in(func(*QueryRequestParams) {}).withRepoDefaults(command.CommandContext{QueryDefaults: command.QueryDefaults{Err: broken}})
	if !errors.Is(err, errcode.RepoSettingsInvalid) || !strings.Contains(err.Error(), "Did you mean enabled?") {
		t.Errorf("err = %v, want repo_settings_invalid with the file's error", err)
	}

	// A host's defaults that Check refuses: refused the same way, the
	// message ending once.
	_, err = in(func(*QueryRequestParams) {}).withRepoDefaults(command.CommandContext{QueryDefaults: command.QueryDefaults{WeekStartDay: "Monday"}})
	if !errors.Is(err, errcode.RepoSettingsInvalid) || !strings.Contains(err.Error(), `query.default.input.weekStartDay is "Monday". Did you mean monday?`) || strings.Contains(err.Error(), "?.") {
		t.Errorf("err = %v, want repo_settings_invalid naming the default", err)
	}
}

// A query's cache.duration over the repo's cap is refused on it, with the cap
// to use; a repo whose default is over its cap refuses every query.
func TestWithRepoDefaultsCap(t *testing.T) {
	capped := &standInCache{settings: command.QueryCacheSettings{MaxDuration: 24 * time.Hour}}
	query := func(duration string) QueryRequestParams {
		return QueryRequestParams{QueryDefinition: QueryDefinition{Dataset: "d", Query: "q"}, Cache: CacheOptions{Duration: duration}}
	}
	cc := command.CommandContext{QueryCache: capped}
	for _, ok := range []string{"", "24h", "10m"} {
		if _, err := query(ok).withRepoDefaults(cc); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	_, err := query("48h").withRepoDefaults(cc)
	v, _ := apperr.DetailsOf(err, apperr.ValidationFailed)
	const want = "cache.duration is 48h, but the repo caches a result for at most 24h (query.cache.max_duration in the repo's settings). Use 24h or less."
	if len(v.Violations) != 1 || v.Violations[0].Field != "cache.duration" || v.Violations[0].Message != want {
		t.Errorf("over the cap: %v, want a violation on cache.duration: %q", err, want)
	}
	// Without a cache of the host's, the cap is anfra's.
	if _, err := query("48h").withRepoDefaults(command.CommandContext{}); err != nil {
		t.Errorf("no cache: %v", err)
	}

	cc.QueryDefaults.CacheDuration = 48 * time.Hour
	if _, err := query("").withRepoDefaults(cc); !errors.Is(err, errcode.RepoSettingsInvalid) || !strings.Contains(err.Error(), "more than query.cache.max_duration (24h)") {
		t.Errorf("a default over the cap: %v, want repo_settings_invalid", err)
	}
}
