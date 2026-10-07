//go:build !embed_sidecar

package canalquery

// Dev builds embed nothing; the sidecar comes from ANFRA_CANAL_QUERY_BIN. This
// lets the repo build without the (large, platform-specific) sidecar asset
// present.
var embedded []byte
