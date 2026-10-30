package sidecar

import (
	"fmt"
	"os"
	"path/filepath"
)

// SocketPath is where a spawned sidecar listens: <name>.sock, in a directory
// of its own that it makes in the temp dir. The temp dir, because a Unix
// socket's path is capped at about a hundred bytes (sun_path), which a deep
// home folder can exceed. A directory of its own, with a random name and
// readable by this user only, because the temp dir is shared with every user
// on the host: at a path another user could guess, they could take the
// address first, and the sidecar would fail to start, or anfra would send its
// requests to them. Its supervisor removes it with RemoveSocket.
func SocketPath(name string) (string, error) {
	dir, err := os.MkdirTemp("", "anfra-")
	if err != nil {
		return "", fmt.Errorf("create %s socket dir: %w", name, err)
	}
	return filepath.Join(dir, name+".sock"), nil
}

// RemoveSocket removes a socket made by SocketPath, and its directory.
func RemoveSocket(path string) {
	if path != "" {
		_ = os.RemoveAll(filepath.Dir(path))
	}
}
