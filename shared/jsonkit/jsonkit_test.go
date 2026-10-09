package jsonkit_test

import (
	"strings"
	"testing"

	"github.com/holistics/anfra/shared/jsonkit"
)

type answer struct {
	Items []string       `json:"items"`
	Attrs map[string]int `json:"attrs"`
	Count int            `json:"count,omitzero"`
	Text  string         `json:"text"`
}

// A value is written as its schema says: a nil list or map is empty, never
// null; a map's keys are sorted, so the same value is the same bytes; invalid
// UTF-8 becomes U+FFFD rather than failing the value.
func TestMarshal(t *testing.T) {
	b, err := jsonkit.Marshal(answer{Text: "a\xffb"})
	if want := `{"items":[],"attrs":{},"text":"a` + "�" + `b"}`; err != nil || string(b) != want {
		t.Errorf("got %s %v, want %s", b, err, want)
	}
	for range 20 {
		b, err := jsonkit.Marshal(map[string]int{"z": 1, "a": 2, "m": 3, "b": 4, "y": 5})
		if want := `{"a":2,"b":4,"m":3,"y":5,"z":1}`; err != nil || string(b) != want {
			t.Fatalf("got %s %v, want %s", b, err, want)
		}
	}
}

func TestMarshalIndent(t *testing.T) {
	b, err := jsonkit.MarshalIndent(map[string]int{"a": 1})
	if want := "{\n  \"a\": 1\n}"; err != nil || string(b) != want {
		t.Errorf("got %q %v, want %q", b, err, want)
	}
}

// A value is read as the contract says: names exactly, each once; invalid UTF-8
// becomes U+FFFD rather than failing the value.
func TestUnmarshal(t *testing.T) {
	var a answer
	if err := jsonkit.Unmarshal([]byte(`{"Text":"x"}`), &a); err != nil || a.Text != "" {
		t.Errorf("a miscased name was read: %+v %v", a, err)
	}
	if err := jsonkit.Unmarshal([]byte(`{"text":"x","text":"y"}`), &a); err == nil {
		t.Error("a duplicate name was accepted")
	}
	if err := jsonkit.Unmarshal([]byte("{\"text\":\"a\xffb\"}"), &a); err != nil || a.Text != "a�b" {
		t.Errorf("invalid UTF-8: got %q %v", a.Text, err)
	}
	if err := jsonkit.UnmarshalRead(strings.NewReader(`{"text":"x"} {}`), &a); err == nil {
		t.Error("data after the value was accepted")
	}
}
