//go:build embed_sidecar && windows

package anfracanal

import (
	_ "embed"

	"github.com/holistics/anfra/internal/sidecar"
)

// Release builds embed anfra-canal, and on Windows DuckDB's own duckdb.dll,
// which it loads: the MotherDuck extension exists only for DuckDB's MSVC build
// (docs/research/windows-canal-build.md). The build pipeline drops both at
// assets/anfra-canal and assets/duckdb.dll before `go build -tags embed_sidecar`.
//
//go:embed assets/anfra-canal
var embedded []byte

//go:embed assets/duckdb.dll
var duckdbDLL []byte

var libs = []sidecar.Library{{Name: "duckdb.dll", Data: duckdbDLL}}
