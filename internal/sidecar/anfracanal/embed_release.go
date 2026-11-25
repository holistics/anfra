//go:build embed_sidecar && !windows

package anfracanal

import (
	_ "embed"

	"github.com/holistics/anfra/internal/sidecar"
)

// Release builds embed anfra-canal. The build pipeline drops the
// platform-specific binary at assets/anfra-canal before
// `go build -tags embed_sidecar`.
//
//go:embed assets/anfra-canal
var embedded []byte

// No libs: on Linux and macOS, anfra-canal links DuckDB in.
var libs []sidecar.Library
