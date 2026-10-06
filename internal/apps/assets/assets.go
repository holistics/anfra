// Package assets holds everything the demo binary carries with it: the built Shell, the Anfra SDK's
// IIFE bundle, the Data App frame bootstrap and, in a distributable build, an anfra binary.
//
// The Shell and SDK bundle are build outputs (see scripts/build.sh), copied under shell/dist and
// sdk/ before `go build`. The committed .gitkeep files keep the embed patterns valid without them.
package assets

import (
	"embed"
	"errors"
	"io/fs"
)

//go:embed all:shell
var shellFS embed.FS

//go:embed all:sdk
var sdkFS embed.FS

//go:embed frame-bootstrap.js
var FrameBootstrap string

// Shell is the built Vite + Vue Shell, or an error when it wasn't built into this binary.
func Shell() (fs.FS, error) {
	dist, err := fs.Sub(shellFS, "shell/dist")
	if err != nil {
		return nil, err
	}
	if _, err := fs.Stat(dist, "index.html"); err != nil {
		return nil, errors.New("the Shell isn't built into this binary: run scripts/build.sh (or `pnpm build:shell`) first")
	}
	return dist, nil
}

// SDKBundle is the Anfra SDK's IIFE bundle, inlined into every Data App.
func SDKBundle() (string, error) {
	b, err := sdkFS.ReadFile("sdk/anfra-sdk.global.js")
	if err != nil {
		return "", errors.New("the Anfra SDK isn't built into this binary: run scripts/build.sh (or `pnpm build:sdk`) first")
	}
	return string(b), nil
}
