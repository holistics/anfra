//go:build embed_sidecar

package anfracanal

import _ "embed"

// Release builds embed anfra-canal. The build pipeline drops the
// platform-specific binary at assets/anfra-canal before
// `go build -tags embed_sidecar`.
//
//go:embed assets/anfra-canal
var embedded []byte
