package main

import (
	"bytes"
	"encoding/json/jsontext"
	"math/big"
	"strconv"
	"strings"

	"github.com/itchyny/gojq"
)

// jqCode is the run's --jq, compiled before the command runs, so an
// expression that doesn't parse fails before anything is done; nil without
// --jq.
var jqCode *gojq.Code

// compileJQ compiles expr, the run's --jq; one that doesn't parse is refused
// as an invalid --jq.
func compileJQ(expr string) error {
	q, err := gojq.Parse(expr)
	if err != nil {
		return refused("--jq", "invalid", "--jq: "+err.Error())
	}
	code, err := gojq.Compile(q)
	if err != nil {
		return refused("--jq", "invalid", "--jq: "+err.Error())
	}
	jqCode = code
	return nil
}

// filterJQ is body, a JSON answer, through --jq: each result as one JSON
// document. A filter that fails on the answer (iterating a string, say) is an
// invalid --jq, as one that doesn't parse.
func filterJQ(body []byte) ([][]byte, error) {
	v, err := decodeJQ(body)
	if err != nil {
		return nil, err
	}
	var out [][]byte
	iter := jqCode.Run(v)
	for {
		r, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := r.(error); ok {
			return nil, refused("--jq", "invalid", "--jq: "+err.Error())
		}
		b, err := gojq.Marshal(r)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

// renderJQ writes body through --jq to w: each result as a JSON line with
// --json; else, when every result is a scalar, each as a plain line, a string
// unquoted, as jq -r writes them, for a shell to read; else as YAML, each a
// document of its own when there are several. gojq's objects are maps, so a
// filtered object's keys come out sorted.
func renderJQ(body []byte, w *bytes.Buffer) error {
	results, err := filterJQ(body)
	if err != nil {
		return err
	}
	if output.json {
		for _, r := range results {
			w.Write(r)
			w.WriteByte('\n')
		}
		return nil
	}
	if allScalars(results) {
		for _, r := range results {
			if s, err := strconv.Unquote(string(r)); err == nil && r[0] == '"' {
				r = []byte(s)
			}
			w.Write(r)
			w.WriteByte('\n')
		}
		return nil
	}
	for _, r := range results {
		y, err := jsonToYAML(r)
		if err != nil {
			return err
		}
		if len(results) > 1 {
			w.WriteString("---\n")
		}
		w.Write(y)
	}
	return nil
}

// allScalars reports whether every JSON document of results is a scalar: a
// string, a number, a boolean or null.
func allScalars(results [][]byte) bool {
	for _, r := range results {
		if len(r) > 0 && (r[0] == '{' || r[0] == '[') {
			return false
		}
	}
	return true
}

// decodeJQ is the JSON document b as the values gojq takes: maps, slices,
// strings, booleans, nil, and numbers as written: an integer as an int, or a
// *big.Int past an int's range, so that every digit is kept; anything else
// as a float64.
func decodeJQ(b []byte) (any, error) {
	return decodeJQValue(jsontext.NewDecoder(bytes.NewReader(b)))
}

func decodeJQValue(dec *jsontext.Decoder) (any, error) {
	tok, err := dec.ReadToken()
	if err != nil {
		return nil, err
	}
	switch tok.Kind() {
	case 'n':
		return nil, nil
	case 't', 'f':
		return tok.Bool(), nil
	case '"':
		return tok.String(), nil
	case '0':
		return jqNumber(tok.String())
	case '[':
		arr := []any{}
		for dec.PeekKind() != ']' {
			v, err := decodeJQValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, v)
		}
		_, err := dec.ReadToken()
		return arr, err
	case '{':
		obj := map[string]any{}
		for dec.PeekKind() != '}' {
			k, err := dec.ReadToken()
			if err != nil {
				return nil, err
			}
			key := k.String() // a token is void once the decoder reads on
			v, err := decodeJQValue(dec)
			if err != nil {
				return nil, err
			}
			obj[key] = v
		}
		_, err := dec.ReadToken()
		return obj, err
	}
	return nil, nil
}

// jqNumber is the JSON number s as gojq takes it, an integer kept whole.
func jqNumber(s string) (any, error) {
	if !strings.ContainsAny(s, ".eE") {
		if n, err := strconv.Atoi(s); err == nil {
			return n, nil
		}
		if n, ok := new(big.Int).SetString(s, 10); ok {
			return n, nil
		}
	}
	return strconv.ParseFloat(s, 64)
}
