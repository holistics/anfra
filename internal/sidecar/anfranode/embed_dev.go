//go:build !embed_sidecar

package anfranode

// Dev builds embed nothing; the sidecar comes from ANFRA_NODE_BIN. This lets
// the repo build without the (large, platform-specific) sidecar asset present.
var embedded []byte
