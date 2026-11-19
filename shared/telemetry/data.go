package telemetry

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// User data in spans is set per category (docs/designs/telemetry.md, data in
// spans): one setting each says whether its attributes are recorded, masked or
// left out. Everything not in a category is anfra's own, and always recorded.

// Category is a kind of user data a span can hold.
type Category string

// The categories. A new one is an entry in Categories, plus one item per
// sidecar that records such data.
const (
	Inputs  Category = "inputs"
	Outputs Category = "outputs"
	SQL     Category = "sql"
	Errors  Category = "errors"
)

// Level is how a category is recorded.
type Level string

// The levels. They mean the same in every category, which offers only those
// that are well defined for it.
const (
	Off    Level = "off"    // nothing recorded
	Masked Level = "masked" // its shape kept, what the user wrote replaced with ?
	Raw    Level = "raw"    // recorded as it is
)

// CategoryDef is one category's entry: everything about it is derived from
// here, its variable, its value check, `anfra status`'s data and the
// sidecars' variable.
type CategoryDef struct {
	Name    Category
	Var     string  // the variable that sets it, ANFRA_TRACE_INPUTS
	Levels  []Level // the values it takes, safest first
	Default Level
	// SidecarVar is the variable a spawned sidecar reads it from; "" when no
	// sidecar records it (SQL, which only anfra records).
	SidecarVar string
}

// Safest is the value DataVar=off holds it at: its first.
func (d CategoryDef) Safest() Level { return d.Levels[0] }

// Categories is the registry of categories, in the order they are shown.
var Categories = []CategoryDef{
	{Name: Inputs, Var: "ANFRA_TRACE_INPUTS", Levels: []Level{Off, Raw}, Default: Off, SidecarVar: "H_OTEL_DATA_INPUTS"},
	{Name: Outputs, Var: "ANFRA_TRACE_OUTPUTS", Levels: []Level{Off, Raw}, Default: Off, SidecarVar: "H_OTEL_DATA_OUTPUTS"},
	{Name: SQL, Var: "ANFRA_TRACE_SQL", Levels: []Level{Off, Masked, Raw}, Default: Masked},
	{Name: Errors, Var: "ANFRA_TRACE_ERRORS", Levels: []Level{Off, Masked, Raw}, Default: Masked, SidecarVar: "H_OTEL_DATA_ERRORS"},
}

// DataVar holds every category at its safest when it is off. It can only take
// data out, so no value turns everything on.
const DataVar = "ANFRA_TRACE_DATA"

// PayloadLimitVar is how many bytes of each payload a span records, at most.
const PayloadLimitVar = "ANFRA_TRACE_PAYLOAD_LIMIT"

// DefaultPayloadLimit is the payload limit unless PayloadLimitVar says
// otherwise: 16 KiB.
const DefaultPayloadLimit = 16 << 10

// Def is c's entry; false for a category the registry doesn't have.
func Def(c Category) (CategoryDef, bool) {
	i := slices.IndexFunc(Categories, func(d CategoryDef) bool { return d.Name == c })
	if i < 0 {
		return CategoryDef{}, false
	}
	return Categories[i], true
}

// Settings say how each category is recorded, and how much of a payload. The
// zero Settings is every category at its default, so a host that sets none
// records what anfra does by default.
type Settings struct {
	levels       map[Category]Level
	payloadLimit int
}

// Defaults is every category at its default.
func Defaults() Settings { return Settings{} }

// NewSettings is levels, each category not named at its default, and limit
// bytes of each payload (0 or less: the default).
func NewSettings(levels map[Category]Level, limit int) Settings {
	s := Settings{levels: map[Category]Level{}, payloadLimit: limit}
	for c, l := range levels {
		s.levels[c] = l
	}
	return s
}

// Level is how c is recorded.
func (s Settings) Level(c Category) Level {
	if l, ok := s.levels[c]; ok && l != "" {
		return l
	}
	if d, ok := Def(c); ok {
		return d.Default
	}
	return Off
}

// PayloadLimit is how many bytes of each payload a span records.
func (s Settings) PayloadLimit() int {
	if s.payloadLimit > 0 {
		return s.payloadLimit
	}
	return DefaultPayloadLimit
}

// Resolved is one setting as it applies, and the variable that set it: "" for
// the default.
type Resolved[T any] struct {
	Value T
	From  string
}

// Resolution is each category as it applies, and the payload limit, with what
// set each: Settings, explained for `anfra status`.
type Resolution struct {
	Data         map[Category]Resolved[Level]
	PayloadLimit Resolved[int]
}

// Settings are the resolution's values.
func (r Resolution) Settings() Settings {
	levels := map[Category]Level{}
	for c, v := range r.Data {
		levels[c] = v.Value
	}
	return NewSettings(levels, r.PayloadLimit.Value)
}

// Resolve reads the settings from getenv (os.Getenv): each category's
// variable, DataVar over them all, and PayloadLimitVar. A value it doesn't
// take is refused (InvalidSettingError), naming the variable.
func Resolve(getenv func(string) string) (Resolution, error) {
	r := Resolution{Data: map[Category]Resolved[Level]{}}
	allOff := false
	switch v := getenv(DataVar); v {
	case "":
	case string(Off):
		allOff = true
	default:
		return Resolution{}, &InvalidSettingError{Name: DataVar, Value: v, Want: []string{string(Off)}}
	}
	for _, d := range Categories {
		v := getenv(d.Var)
		if v != "" && !slices.Contains(d.Levels, Level(v)) {
			return Resolution{}, &InvalidSettingError{Name: d.Var, Value: v, Want: levelNames(d.Levels)}
		}
		switch {
		case allOff:
			r.Data[d.Name] = Resolved[Level]{Value: d.Safest(), From: DataVar}
		case v != "":
			r.Data[d.Name] = Resolved[Level]{Value: Level(v), From: d.Var}
		default:
			r.Data[d.Name] = Resolved[Level]{Value: d.Default}
		}
	}
	r.PayloadLimit = Resolved[int]{Value: DefaultPayloadLimit}
	if v := getenv(PayloadLimitVar); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return Resolution{}, &InvalidSettingError{Name: PayloadLimitVar, Value: v, Want: []string{"a number of bytes, more than 0"}}
		}
		r.PayloadLimit = Resolved[int]{Value: n, From: PayloadLimitVar}
	}
	return r, nil
}

// SidecarEnv is s as the variables a spawned sidecar reads, NAME=value, one per
// category a sidecar records.
func SidecarEnv(s Settings) []string {
	var env []string
	for _, d := range Categories {
		if d.SidecarVar != "" {
			env = append(env, d.SidecarVar+"="+string(s.Level(d.Name)))
		}
	}
	return env
}

func levelNames(levels []Level) []string {
	names := make([]string, len(levels))
	for i, l := range levels {
		names[i] = string(l)
	}
	return names
}

// InvalidSettingError is a telemetry setting set to a value it doesn't take.
type InvalidSettingError struct {
	Name  string   // the variable, ANFRA_TRACE_SQL
	Value string   // what it is set to
	Want  []string // what it takes
}

func (e *InvalidSettingError) Error() string {
	return fmt.Sprintf("%s must be %s, not %q", e.Name, orList(e.Want), e.Value)
}

// orList is "a", "a or b", "a, b or c".
func orList(words []string) string {
	if len(words) <= 1 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + " or " + words[len(words)-1]
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
