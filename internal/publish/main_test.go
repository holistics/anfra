package publish

import (
	"os"
	"testing"

	"github.com/holistics/anfra/shared/apperr"
)

// The tests run as publish does in the CLI by default (cmd/anfra's main),
// where an error's causes are shown.
func TestMain(m *testing.M) {
	apperr.DisableErrorEncapsulation()
	os.Exit(m.Run())
}
