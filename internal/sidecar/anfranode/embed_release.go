//go:build embed_sidecar

package anfranode

import _ "embed"

// Release builds embed the compiled sidecar. The build pipeline drops the
// platform-specific binary at assets/anfra-node before
// `go build -tags embed_sidecar`.
//
//go:embed assets/anfra-node
var embedded []byte
