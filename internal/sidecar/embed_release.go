//go:build embed_sidecar

package sidecar

import _ "embed"

// Release builds embed the compiled sidecars. The build pipeline drops the
// platform-specific binaries at assets/anfra-node and assets/canal-query before
// `go build -tags embed_sidecar`.

//go:embed assets/anfra-node
var anfraNodeBin []byte

//go:embed assets/canal-query
var canalQueryBin []byte

func embeddedAnfraNode() ([]byte, bool)  { return anfraNodeBin, len(anfraNodeBin) > 0 }
func embeddedCanalQuery() ([]byte, bool) { return canalQueryBin, len(canalQueryBin) > 0 }
