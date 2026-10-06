// Package apps is Data App serving: `anfra serve --apps` serves the Repo's Data Apps to a browser
// through the Shell, running their queries in-process on the server's warm sidecars.
package apps

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/holistics/anfra/internal/apps/assets"
	"github.com/holistics/anfra/internal/apps/dispatch"
	"github.com/holistics/anfra/internal/apps/repofiles"
	"github.com/holistics/anfra/internal/apps/server"
)

// Built is the Shell and the Anfra SDK bundle this binary carries.
type Built struct {
	Shell     fs.FS
	SDKBundle string
}

// Check reports what would stop Data App serving before anything starts: a binary without the
// Shell built in, a Repo without the config anfra reads Datasets from, or a database that isn't
// listening. It returns what the binary carries, and a warning for each Data App it will leave out.
func Check(repoDir string) (Built, []string, error) {
	shell, err := assets.Shell()
	if err != nil {
		return Built{}, nil, err
	}
	sdkBundle, err := assets.SDKBundle()
	if err != nil {
		return Built{}, nil, err
	}
	if err := repofiles.CheckDataSources(repoDir); err != nil {
		return Built{}, nil, err
	}
	if err := repofiles.CheckContextSources(repoDir); err != nil {
		return Built{}, nil, err
	}
	var warnings []string
	for _, name := range repofiles.ReservedApps(repoDir) {
		warnings = append(warnings, fmt.Sprintf("ignoring Data App %s: /%s is reserved for anfra's own routes", name, repofiles.ReservedName))
	}
	return Built{Shell: shell, SDKBundle: sdkBundle}, warnings, nil
}

// Listen claims the Shell's port on 127.0.0.1, so a taken port fails before anything else starts.
func Listen(port int) (net.Listener, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("can't serve Data Apps on port %d: %w", port, err)
	}
	return ln, nil
}

// Serve serves the Shell on ln until ctx ends, watching the Repo for changes to its Data Apps and AML.
func Serve(ctx context.Context, ln net.Listener, caller dispatch.Caller, repoDir string, built Built) error {
	srv, err := server.New(ctx, caller, repoDir, built.SDKBundle, built.Shell)
	if err != nil {
		return err
	}
	stopWatching, err := repofiles.Watch(repoDir, srv.OnChange)
	if err != nil {
		return err
	}
	defer stopWatching()

	httpServer := &http.Server{Handler: srv, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		// Close, not Shutdown: open Shells hold event streams that would never drain.
		srv.Close()
		_ = httpServer.Close()
	}()
	if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
