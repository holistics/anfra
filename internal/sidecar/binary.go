package sidecar

import (
	"fmt"
	"os"
)

// Binary returns a path to the sidecar name's executable. Release builds embed
// it (embedded, extracted to a content-addressed cache dir); dev builds embed
// nothing, and take it from the environment variable env.
func Binary(name, env string, embedded []byte) (string, error) {
	if len(embedded) > 0 {
		return extractRuntime(name, embedded)
	}
	if path := os.Getenv(env); path != "" {
		return path, nil
	}
	return "", fmt.Errorf("this build of anfra doesn't include %[1]s. Set %[2]s to the path of %[1]s's binary", name, env)
}

// Unpack writes the embedded binary name to anfra's folder, as Binary does, and
// returns its path: for a sidecar whose binary Binary's one variable can't
// choose.
func Unpack(name string, embedded []byte) (string, error) {
	return extractRuntime(name, embedded)
}

// unpackErr names the step that failed and where anfra's folder comes from,
// whichever file operation failed: the OS error already names the path, and a
// reader who can't write there (a full disk, a read-only home) may move the
// folder with ANFRA_HOME.
func unpackErr(name string, err error) error {
	return fmt.Errorf("unpack %s into anfra's folder (ANFRA_HOME): %w", name, err)
}
