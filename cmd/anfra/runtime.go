package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/shared/jsonkit"
)

// runtimeFile is a running `anfra serve`, as it records itself in its repo's
// runtime directory: how the CLI and other tools find it, without a port to
// know. Owner-only. Written when the server is ready, removed when it stops; a
// crash leaves it behind, which is why a reader checks it (findServer).
type runtimeFile struct {
	PID        int       `json:"pid"`
	URL        string    `json:"url"`
	RepoID     string    `json:"repo_id"`
	RepoDir    string    `json:"repo_dir"`
	InstanceID string    `json:"instance_id"`
	Version    string    `json:"version"`
	StartedAt  time.Time `json:"started_at"`
	// Config is what the server started with that a command could see
	// differently: for a later check that a running server still matches
	// (serve_daemon.md).
	Config runtimeConfig `json:"config"`
}

type runtimeConfig struct {
	NodeURL       string `json:"node_url,omitempty"`
	CanalQueryURL string `json:"canal_query_url,omitempty"`
}

func runtimePath(r repo.Repo) string { return filepath.Join(r.RuntimeDir(), "serve.json") }

func writeRuntime(r repo.Repo, f runtimeFile) error {
	if err := os.MkdirAll(r.RuntimeDir(), 0o700); err != nil {
		return err
	}
	b, err := jsonkit.MarshalIndent(f)
	if err != nil {
		return err
	}
	// Written whole, then renamed, so a reader never sees half of it.
	tmp := runtimePath(r) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, runtimePath(r))
}

func readRuntime(path string) (runtimeFile, bool) {
	b, err := os.ReadFile(path) //nolint:gosec // G304: our own state directory
	if err != nil {
		return runtimeFile{}, false
	}
	var f runtimeFile
	if jsonkit.Unmarshal(b, &f) != nil {
		return runtimeFile{}, false
	}
	return f, true
}

// removeRuntime removes the repo's runtime file if it is still this server's: a
// newer server's is left alone.
func removeRuntime(r repo.Repo, instanceID string) {
	if f, ok := readRuntime(runtimePath(r)); ok && f.InstanceID == instanceID {
		_ = os.Remove(runtimePath(r))
	}
}

// health is what /health answers: who the server is, so a client can tell it
// from another server that took over the port after a crash.
type health struct {
	Status     string `json:"status"`
	RepoID     string `json:"repo_id"`
	InstanceID string `json:"instance_id"`
	Version    string `json:"version"`
}

// findServer is the repo's running server: the one its runtime file names, if
// it answers as that server — this repo, this instance. Anything else (no file,
// no answer, another server on the port) is no server, and the CLI runs the
// command itself.
func findServer(ctx context.Context, r repo.Repo) (runtimeFile, bool) {
	f, ok := readRuntime(runtimePath(r))
	if !ok || f.RepoID != r.ID {
		return runtimeFile{}, false
	}
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL+"/health", nil)
	if err != nil {
		return runtimeFile{}, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return runtimeFile{}, false
	}
	defer resp.Body.Close()
	var h health
	if resp.StatusCode != http.StatusOK || jsonkit.UnmarshalRead(resp.Body, &h) != nil ||
		h.RepoID != f.RepoID || h.InstanceID != f.InstanceID {
		return runtimeFile{}, false
	}
	return f, true
}

// holderOf is the repo whose server records addr (host:port) in its runtime
// file, for an error that says who holds a port. "" when none does.
func holderOf(r repo.Repo, addr string) string {
	paths, _ := filepath.Glob(filepath.Join(filepath.Dir(r.DataDir), "*", "runtime", "serve.json"))
	for _, p := range paths {
		if f, ok := readRuntime(p); ok && f.URL == "http://"+addr {
			return f.RepoDir
		}
	}
	return ""
}

func newInstanceID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(errors.New("crypto/rand failed: " + err.Error())) // never, per its docs
	}
	return hex.EncodeToString(b)
}
