// Package robustio is file operations that hold up on Windows, where a file
// another process has open can't be renamed over, as Go's own
// cmd/go/internal/robustio does for its tools.
package robustio
