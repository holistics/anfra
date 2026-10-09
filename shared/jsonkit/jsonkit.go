// Package jsonkit is how anfra reads and writes JSON: encoding/json/v2 with one
// set of options, used by every package, so a value is encoded the same way
// whichever door it leaves by. Don't import encoding/json or encoding/json/v2
// elsewhere; depguard refuses it. See docs/designs/json.md.
package jsonkit

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"io"
)

// encoding is how a value is written. Beyond v2's defaults, which write a nil
// slice or map as [] or {}, as a schema says, never null:
//   - Deterministic: a map's keys sorted, so the same value is the same bytes.
//   - AllowInvalidUTF8: a string's invalid UTF-8 becomes U+FFFD, rather than
//     failing the whole value over one bad byte.
var encoding = json.JoinOptions(json.Deterministic(true), jsontext.AllowInvalidUTF8(true))

// decoding is how a value is read: v2's defaults (names matched exactly,
// duplicate names refused), and invalid UTF-8 becomes U+FFFD, so a bad byte in
// a sidecar's answer doesn't fail it.
var decoding = jsontext.AllowInvalidUTF8(true)

// Marshal is v as JSON.
func Marshal(v any) ([]byte, error) { return json.Marshal(v, encoding) }

// MarshalIndent is v as JSON, indented by two spaces, for a file people read.
func MarshalIndent(v any) ([]byte, error) {
	return json.Marshal(v, encoding, jsontext.WithIndent("  "))
}

// MarshalWrite writes v as JSON to w.
func MarshalWrite(w io.Writer, v any) error { return json.MarshalWrite(w, v, encoding) }

// MarshalEncode writes v to enc, with enc's options: for a MarshalJSONTo method,
// so the options of whatever is encoding it reach inside.
func MarshalEncode(enc *jsontext.Encoder, v any) error { return json.MarshalEncode(enc, v) }

// Unmarshal reads the JSON value b into v.
func Unmarshal(b []byte, v any) error { return json.Unmarshal(b, v, decoding) }

// Valid reports whether b is one JSON value, as Unmarshal would read it.
func Valid(b []byte) bool { return jsontext.Value(b).IsValid(decoding) }

// UnmarshalRead reads the one JSON value in r, to its end, into v.
func UnmarshalRead(r io.Reader, v any) error { return json.UnmarshalRead(r, v, decoding) }
