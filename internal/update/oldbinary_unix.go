//go:build !windows

package update

// installCommand installs anfra by hand, for when an update can't.
const installCommand = "curl -fsSL https://anfra.ai/install.sh | bash"

// oldSavePath is "": selfupdate's default. Unix replaces a running binary in
// place, so nothing is left behind.
func oldSavePath(string) string { return "" }
