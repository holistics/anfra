//go:build !embed_sidecar

package sidecar

// Dev builds embed nothing; the sidecar comes from ANFRA_NODE_BIN. This lets
// the repo build without the (large, platform-specific) sidecar asset present.
func embeddedAnfraNode() ([]byte, bool)  { return nil, false }
func embeddedCanalQuery() ([]byte, bool) { return nil, false }
