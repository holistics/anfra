package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/repo"
)

// A console's server is the repo's warm anfra serve
// (docs/designs/console.md#the-rest): one already running is used and left
// alone; otherwise the console starts one when the session first needs the
// query engine (entering a mode that runs on it, or a command that does),
// and stops it when the session ends, releasing what it holds, a DuckDB
// file's lock among them.
const (
	// consoleServerIdle stops a server the console started after this long
	// without a request, should the console die without stopping it.
	consoleServerIdle = 10 * time.Minute
	// consoleServerWait is how long a command waits for a server starting,
	// before running on its own, as at a shell.
	consoleServerWait = 30 * time.Second
)

// consoleServer is the session's server, as the console knows it.
type consoleServer struct {
	repoDir string // "" outside an anfra repo: no server

	mu       sync.Mutex
	url      string        // the server's, once it answers; "" before
	started  *os.Process   // the server this console started; nil for one it found
	starting chan struct{} // closed when a start ends, ready or not; nil when none is under way
}

func newConsoleServer() *consoleServer {
	return &consoleServer{repoDir: repoDirIfAny()}
}

// state is the server as the line under the frame shows it: "starting",
// "warm", or "" for none.
func (cs *consoleServer) state() string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	switch {
	case cs.starting != nil:
		return "starting"
	case cs.url != "":
		return "warm"
	}
	return ""
}

// describe is \?'s line for the server: what it is, where, and whose.
func (cs *consoleServer) describe() string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	switch {
	case cs.starting != nil:
		return "starting"
	case cs.url != "" && cs.started != nil:
		return "running at " + cs.url + ", started by this console"
	case cs.url != "":
		return "running at " + cs.url + ", started outside this console"
	case cs.repoDir == "":
		return "none, because this folder isn't an anfra repo"
	}
	return "none yet. One starts when you run a query, or enter a mode that needs it."
}

// ensure starts the server in the background, unless one runs, is starting,
// or there's no repo: for a mode about to need it.
func (cs *consoleServer) ensure(ctx context.Context) {
	if cs.repoDir == "" {
		return
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.starting != nil {
		return
	}
	if f, ok := findServer(ctx, repo.Resolve(cs.repoDir)); ok {
		cs.url = f.URL
		return
	}
	cs.url = ""
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, "serve", "--idle-timeout", consoleServerIdle.String()) //nolint:gosec // our own binary, fixed args
	cmd.Dir = cs.repoDir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil // it logs to the repo's log
	detachProcess(cmd)
	if err := cmd.Start(); err != nil {
		return
	}
	cs.started = cmd.Process
	done := make(chan struct{})
	cs.starting = done
	go func() { _ = cmd.Wait() }() // reaped when it stops
	go func() {
		defer close(done)
		deadline := time.Now().Add(consoleServerWait)
		for time.Now().Before(deadline) {
			if f, ok := findServer(context.WithoutCancel(ctx), repo.Resolve(cs.repoDir)); ok {
				cs.mu.Lock()
				cs.url, cs.starting = f.URL, nil
				cs.mu.Unlock()
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
		cs.mu.Lock()
		cs.starting = nil
		cs.mu.Unlock()
	}()
}

// ready makes sure the server answers before a command that runs on the
// query engine: starting it if none runs, and waiting for it, with a
// spinner, while it starts; a command then runs on its own if it didn't come
// up. The command's own forwarding finds it (invoke).
func (cs *consoleServer) ready(ctx context.Context, out io.Writer) {
	if cs.repoDir == "" {
		return
	}
	if cs.state() == "warm" {
		if _, ok := findServer(ctx, repo.Resolve(cs.repoDir)); ok {
			return
		}
		cs.mu.Lock()
		cs.url = "" // it stopped, idle say: start it again
		cs.mu.Unlock()
	}
	cs.ensure(ctx)
	cs.mu.Lock()
	starting := cs.starting
	cs.mu.Unlock()
	if starting == nil {
		return
	}
	_ = cliui.RunActivity(ctx, os.Stdin, out, "Starting a server for this repo…", func(ctx context.Context, _ func(string), _ func(string)) error {
		select {
		case <-starting:
		case <-ctx.Done():
		}
		return nil
	})
}

// stop stops a server the console started, as the session ends; one it found
// running is left alone.
func (cs *consoleServer) stop() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.started != nil {
		_ = cs.started.Signal(syscall.SIGTERM)
		cs.started, cs.url = nil, ""
	}
}
