//go:build embed_sidecar

package canalquery

import _ "embed"

// Release builds embed the compiled sidecar. The build pipeline drops the
// platform-specific binary at assets/canal-query before
// `go build -tags embed_sidecar`.
//
//go:embed assets/canal-query
var embedded []byte
