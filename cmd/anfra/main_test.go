package main

import (
	"os"
	"testing"

	"github.com/holistics/anfra/shared/apperr"
)

// The tests run as the binary does by default (main): causes shown, since
// ANFRA_HIDE_ERROR_CAUSES is unset.
func TestMain(m *testing.M) {
	apperr.DisableErrorEncapsulation()
	os.Exit(m.Run())
}
