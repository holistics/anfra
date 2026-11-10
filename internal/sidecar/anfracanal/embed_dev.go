//go:build !embed_sidecar

package anfracanal

// Dev builds embed nothing; the binary comes from ANFRA_CANAL_BIN, or a service's own from its variable (Command).
// This lets the repo build without the (large, platform-specific) asset present.
var embedded []byte
