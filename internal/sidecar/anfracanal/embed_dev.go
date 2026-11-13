//go:build !embed_sidecar

package anfracanal

// Builds from source embed nothing; anfra-canal comes from ANFRA_CANAL_BIN, or
// a standalone service from its own variable (Command). This lets the repo
// build without the (large, platform-specific) binary present.
var embedded []byte
