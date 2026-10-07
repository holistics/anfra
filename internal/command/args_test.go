package command

import (
	"reflect"
	"testing"
)

// A malformed In is refused when the command is defined, not when it runs.
func TestParseArgsRefusesMistakes(t *testing.T) {
	for name, in := range map[string]any{
		"an unsupported type": struct {
			N float64 `json:"n,omitempty" doc:"x"`
		}{},
		"a required int": struct {
			N int `json:"n" doc:"x"`
		}{},
		"a positional object": struct {
			O *struct{} `json:"o,omitempty" cli:"positional" doc:"x"`
		}{},
		"an embedded pointer": struct {
			*embedded
		}{},
		"no doc": struct {
			S string `json:"s,omitempty"`
		}{},
		"a required bool": struct {
			B bool `json:"b" doc:"x"`
		}{},
		"a required string with a default": struct {
			S string `json:"s" default:"a" doc:"x"`
		}{},
		"a default outside its enum": struct {
			S string `json:"s,omitempty" enum:"a,b" default:"c" doc:"x"`
		}{},
		"a group of one": struct {
			S string `json:"s,omitempty" group:"g" doc:"x"`
		}{},
		"a repeated name": struct {
			A string `json:"a,omitempty" doc:"x"`
			B string `json:"b,omitempty" alias:"a" doc:"x"`
		}{},
		"a long shorthand": struct {
			S string `json:"s,omitempty" short:"ss" doc:"x"`
		}{},
		"two positionals": struct {
			A string `json:"a,omitempty" cli:"positional" doc:"x"`
			B string `json:"b,omitempty" cli:"positional" doc:"x"`
		}{},
		"help, which every command has": struct {
			H bool `json:"help,omitempty" doc:"x"`
		}{},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseArgs(reflect.TypeOf(in)); err == nil {
				t.Errorf("%s was accepted", name)
			}
		})
	}
}

type embedded struct {
	S string `json:"s,omitempty" doc:"x"`
}
