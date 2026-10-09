package command

import (
	"reflect"
	"strings"
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
		"an object field the CLI sets, with no doc": struct {
			O *struct {
				S string `json:"s,omitempty"`
			} `json:"o,omitempty" doc:"x"`
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

// An object arg's string, bool, int and []string fields are set one by one; an
// object or a list of them inside it is left to the arg's JSON.
func TestObjectArgFields(t *testing.T) {
	args, err := parseArgs(reflect.TypeFor[struct {
		O *struct {
			Header string     `json:"header,omitempty" enum:"labels,names" doc:"the header"`
			Rows   int        `json:"rows,omitempty" doc:"rows"`
			BOM    bool       `json:"bom,omitempty" doc:"a BOM"`
			Tags   []string   `json:"tags,omitempty" doc:"tags"`
			Nested *struct{}  `json:"nested,omitempty"`
			List   []struct{} `json:"list,omitempty"`
		} `json:"o,omitempty" doc:"options"`
	}]())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range args[0].Fields {
		got = append(got, f.Name+":"+string(f.Type))
	}
	if want := "header:string rows:int bom:bool tags:string_array"; strings.Join(got, " ") != want {
		t.Errorf("fields = %s, want %s", strings.Join(got, " "), want)
	}
	if e := args[0].Fields[0].Enum; strings.Join(e, ",") != "labels,names" {
		t.Errorf("header's enum = %v", e)
	}
}
