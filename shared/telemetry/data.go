package telemetry

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"
)

// The user's data in spans is one switch (docs/designs/telemetry.md, data in
// spans): on, it is recorded as it is; off, it is left out. Everything that
// isn't data is anfra's own, and always recorded.

// DataVar is the switch that records the user's data in spans: 1 or 0, on
// unless set to 0. The CLI warns when it is on by default and traces leave the
// machine (internal/telemetry, GuardWarnings).
const DataVar = "ANFRA_EXPERIMENTAL_TRACE_DATA"

// SidecarVar is DataVar as a spawned sidecar reads it, resolved: 1 or 0.
const SidecarVar = "H_OTEL_DATA"

// PayloadLimitVar is how many bytes of each payload a span records, at most.
const PayloadLimitVar = "ANFRA_EXPERIMENTAL_TRACE_PAYLOAD_LIMIT"

// BaggageIncludeVar names the members of a caller's baggage that get into
// anfra, recorded as the caller's ids and sent on to its sidecars: keys, or
// patterns with * and ?, separated by commas, such as "tenant.id,user.*", or
// NoBaggage (AllowedBaggage).
const BaggageIncludeVar = "ANFRA_EXPERIMENTAL_TRACE_BAGGAGE_INCLUDE"

// NoBaggage is BaggageIncludeVar letting no baggage in at all. Empty can't
// say it: an empty variable is an unset one, which is the default.
const NoBaggage = "none"

// DefaultBaggageInclude is what BaggageIncludeVar is unless set: the caller's
// session alone.
var DefaultBaggageInclude = []string{"gen_ai.conversation.id"}

// maxBaggagePatterns bounds BaggageIncludeVar.
const maxBaggagePatterns = 16

// DefaultPayloadLimit is the payload limit unless PayloadLimitVar says
// otherwise: 16 KiB.
const DefaultPayloadLimit = 16 << 10

// Settings say whether spans record the user's data, and how much of a
// payload. The zero Settings records none, so a host that embeds the engine and
// sets none records no user data; the CLI and serve resolve theirs (Resolve).
type Settings struct {
	data           bool
	payloadLimit   int
	baggageInclude []string // nil: DefaultBaggageInclude
}

// Defaults records no user data.
func Defaults() Settings { return Settings{} }

// NewSettings records the user's data when data is set, and limit bytes of
// each payload (0 or less: the default).
func NewSettings(data bool, limit int) Settings {
	return Settings{data: data, payloadLimit: limit}
}

// RecordsData reports whether spans record the user's data.
func (s Settings) RecordsData() bool { return s.data }

// PayloadLimit is how many bytes of each payload a span records.
func (s Settings) PayloadLimit() int {
	if s.payloadLimit > 0 {
		return s.payloadLimit
	}
	return DefaultPayloadLimit
}

// WithBaggageInclude is s letting in the members of a caller's baggage that
// patterns name (BaggageIncludeVar), instead of DefaultBaggageInclude. Empty
// patterns let none in.
func (s Settings) WithBaggageInclude(patterns []string) Settings {
	s.baggageInclude = append([]string{}, patterns...)
	return s
}

// BaggageInclude is the patterns of the caller's baggage members that get in
// (AllowedBaggage).
func (s Settings) BaggageInclude() []string {
	if s.baggageInclude == nil {
		return DefaultBaggageInclude
	}
	return s.baggageInclude
}

// Resolved is one setting as it applies, and the variable that set it: "" for
// the default.
type Resolved[T any] struct {
	Value T
	From  string
}

// Resolution is whether data is recorded, the payload limit and the baggage
// recorded, with what set each: Settings, explained for `anfra status`.
type Resolution struct {
	Data           Resolved[bool]
	PayloadLimit   Resolved[int]
	BaggageInclude Resolved[[]string]
}

// Settings are the resolution's values.
func (r Resolution) Settings() Settings {
	s := NewSettings(r.Data.Value, r.PayloadLimit.Value)
	if r.BaggageInclude.From != "" {
		s = s.WithBaggageInclude(r.BaggageInclude.Value)
	}
	return s
}

// Resolve reads the settings from getenv (os.Getenv): DataVar, on unless 0,
// PayloadLimitVar and BaggageIncludeVar. A value it doesn't take is refused
// (InvalidSettingError), naming the variable.
func Resolve(getenv func(string) string) (Resolution, error) {
	r := Resolution{Data: Resolved[bool]{Value: true}}
	switch v := getenv(DataVar); v {
	case "":
	case "0", "1":
		r.Data = Resolved[bool]{Value: v == "1", From: DataVar}
	default:
		return Resolution{}, &InvalidSettingError{Name: DataVar, Value: v, Want: "1 or 0"}
	}
	r.PayloadLimit = Resolved[int]{Value: DefaultPayloadLimit}
	if v := getenv(PayloadLimitVar); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Resolution{}, &InvalidSettingError{Name: PayloadLimitVar, Value: v, Want: "a number of bytes, more than 0"}
		}
		r.PayloadLimit = Resolved[int]{Value: n, From: PayloadLimitVar}
	}
	r.BaggageInclude = Resolved[[]string]{Value: DefaultBaggageInclude}
	if v, set := lookup(getenv, BaggageIncludeVar); set {
		patterns := []string{}
		if strings.TrimSpace(v) != NoBaggage {
			var err error
			if patterns, err = parseBaggageInclude(v); err != nil {
				return Resolution{}, &InvalidSettingError{Name: BaggageIncludeVar, Value: v,
					Want: fmt.Sprintf("%q, or at most %d keys or patterns, such as tenant.id,user.*, separated by commas", NoBaggage, maxBaggagePatterns)}
			}
		}
		r.BaggageInclude = Resolved[[]string]{Value: patterns, From: BaggageIncludeVar}
	}
	return r, nil
}

// lookup is getenv's value for name, and whether it is set: getenv gives ""
// for a variable that isn't, so a value that is only spaces counts as unset.
func lookup(getenv func(string) string, name string) (string, bool) {
	v := getenv(name)
	return v, strings.TrimSpace(v) != ""
}

// parseBaggageInclude is BaggageIncludeVar's patterns.
func parseBaggageInclude(v string) ([]string, error) {
	var patterns []string
	for p := range strings.SplitSeq(v, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, err := path.Match(p, ""); err != nil || strings.ContainsAny(p, "/[]\\ ") || p == NoBaggage {
			return nil, fmt.Errorf("pattern %q", p)
		}
		patterns = append(patterns, p)
	}
	if len(patterns) == 0 || len(patterns) > maxBaggagePatterns {
		return nil, fmt.Errorf("%d patterns", len(patterns))
	}
	return patterns, nil
}

// SidecarEnv is s as the variable a spawned sidecar reads: H_OTEL_DATA=1 or 0.
func SidecarEnv(s Settings) []string {
	if s.RecordsData() {
		return []string{SidecarVar + "=1"}
	}
	return []string{SidecarVar + "=0"}
}

// InvalidSettingError is a telemetry setting set to a value it doesn't take.
type InvalidSettingError struct {
	Name  string // the variable, ANFRA_EXPERIMENTAL_TRACE_DATA
	Value string // what it is set to
	Want  string // what it takes
}

func (e *InvalidSettingError) Error() string {
	return fmt.Sprintf("%s must be %s, not %q", e.Name, e.Want, e.Value)
}

type settingsKey struct{}

// WithSettings is ctx with s, for every span started in it: the host's for the
// call (docs/designs/telemetry.md). A span started without them follows the
// settings its span processor was given (Filter).
func WithSettings(ctx context.Context, s Settings) context.Context {
	return context.WithValue(ctx, settingsKey{}, s)
}

// SettingsFrom is what WithSettings put in ctx, and whether it put any.
func SettingsFrom(ctx context.Context) (Settings, bool) {
	s, ok := ctx.Value(settingsKey{}).(Settings)
	return s, ok
}

// SettingsOf is ctx's settings, or the defaults.
func SettingsOf(ctx context.Context) Settings {
	s, _ := SettingsFrom(ctx)
	return s
}
