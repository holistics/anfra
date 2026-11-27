//go:build !windows

package main

import (
	"io"

	"github.com/muesli/cancelreader"
)

// newLineReader is a reader of typed lines that ctx's end can stop.
func newLineReader(in io.Reader) (cancelreader.CancelReader, error) {
	return cancelreader.NewReader(in)
}
