package main

import (
	"fmt"
	"io"
)

// renderPretty writes v for a person (docs/designs/cli-pretty.md). Not built
// yet: YAML until it is.
func renderPretty(w io.Writer, v any, _ int) error {
	_ = fmt.Sprint
	return writeStructured(w, v)
}
