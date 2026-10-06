// Package anfra runs the demo's own `anfra serve --socket` and calls its commands over `/call` on that socket.
package anfra

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// StartupError is a startup check that failed, with a message for the person running the demo.
type StartupError struct{ Message string }

func (e *StartupError) Error() string { return e.Message }

// Startupf builds a StartupError.
func Startupf(format string, args ...any) error {
	return &StartupError{Message: fmt.Sprintf(format, args...)}
}

// CallError is anfra's `/call` answering with an error: a bad query, an unknown dataset, …
type CallError struct {
	Message string
	Status  int
}

func (e *CallError) Error() string { return e.Message }

// Resolve finds the anfra executable the way a shell would: a path as given, or a name on PATH.
func Resolve(bin string) (string, error) {
	if strings.ContainsRune(bin, os.PathSeparator) {
		abs, _ := filepath.Abs(bin)
		if info, err := os.Stat(abs); err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			return abs, nil
		}
		return "", Startupf("The anfra binary %q doesn't exist or isn't executable. Set ANFRA_BIN to your anfra build.", bin)
	}
	found, err := exec.LookPath(bin)
	if err != nil {
		return "", Startupf("Couldn't find %q on PATH. Set ANFRA_BIN to the path of your anfra build.", bin)
	}
	return found, nil
}

// Extract writes an embedded anfra binary to the user cache, once per build, and returns its path.
func Extract(binary []byte) (string, error) {
	sum := sha256.Sum256(binary)
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	file := filepath.Join(dir, "anfra-demo", "anfra-"+hex.EncodeToString(sum[:6]))
	if info, err := os.Stat(file); err == nil && info.Size() == int64(len(binary)) {
		return file, nil
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return "", err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, binary, 0o755); err != nil {
		return "", err
	}
	return file, os.Rename(tmp, file)
}

// maxSocketPath is the longest Unix socket path that works on every platform the demo
// ships for: sun_path is 104 bytes on macOS (108 on Linux), including the trailing NUL.
const maxSocketPath = 103

// tail keeps the last few KB a process wrote, to explain a failed start.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > 4000 {
		t.buf = t.buf[len(t.buf)-4000:]
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if s := strings.TrimSpace(string(t.buf)); s != "" {
		return s
	}
	return "(no output)"
}

// Anfra is a running `anfra serve --socket`, started with the Data Folder as its working directory.
type Anfra struct {
	Socket string
	cmd    *exec.Cmd
	done   chan struct{}
	http   *http.Client

	mu     sync.Mutex
	onExit []func()
}

// Options says which anfra to run, on which Data Folder.
type Options struct {
	Bin        string
	DataFolder string
	Ready      time.Duration
}

// Start launches anfra on a socket in a fresh temp dir and waits until it answers /health.
func Start(opts Options) (*Anfra, error) {
	if opts.Ready == 0 {
		opts.Ready = 30 * time.Second
	}

	dir, err := os.MkdirTemp("", "anfra-demo-")
	if err != nil {
		return nil, err
	}
	socket := filepath.Join(dir, "anfra.sock")
	if len(socket) > maxSocketPath {
		_ = os.RemoveAll(dir)
		return nil, Startupf("The socket path for anfra, %s, is longer than %d bytes. Set TMPDIR to a shorter directory.", socket, maxSocketPath)
	}

	cmd := exec.Command(opts.Bin, "serve", "--socket", socket)
	cmd.Dir = opts.DataFolder
	cmd.Env = append(os.Environ(), "ANFRA_NO_UPDATE_NOTIFIER=1")
	out := &tail{}
	cmd.Stdout, cmd.Stderr = out, out
	setParentDeathSignal(cmd)
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(dir)
		return nil, Startupf("Couldn't start anfra (%s): %v", opts.Bin, err)
	}

	a := &Anfra{
		Socket: socket,
		cmd:    cmd,
		done:   make(chan struct{}),
		http: &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socket)
			},
		}},
	}
	go func() {
		_ = cmd.Wait()
		_ = os.RemoveAll(dir) // the socket's dir: anfra is gone, so is its socket

		close(a.done)
		a.mu.Lock()
		listeners := a.onExit
		a.mu.Unlock()
		for _, f := range listeners {
			f()
		}
	}()

	deadline := time.Now().Add(opts.Ready)
	for time.Now().Before(deadline) {
		select {
		case <-a.done:
			if strings.Contains(out.String(), "unknown flag: --socket") {
				return nil, Startupf("anfra (%s) is too old: it has no `serve --socket`. Rebuild it from a checkout that has it.\n%s", opts.Bin, out)
			}
			return nil, Startupf("anfra exited during startup:\n%s", out)
		default:
		}
		if a.Healthy(context.Background()) {
			return a, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	a.Stop()
	return nil, Startupf("anfra didn't become healthy within %s:\n%s", opts.Ready, out)
}

// Running reports whether the anfra process is still alive.
func (a *Anfra) Running() bool {
	select {
	case <-a.done:
		return false
	default:
		return true
	}
}

// OnExit registers f to run when the anfra process exits.
func (a *Anfra) OnExit(f func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.onExit = append(a.onExit, f)
}

// Healthy reports whether anfra answers /health.
func (a *Anfra) Healthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://anfra/health", nil)
	res, err := a.http.Do(req)
	if err != nil {
		return false
	}
	_ = res.Body.Close()
	return res.StatusCode == http.StatusOK
}

// Call runs one anfra command over /call and returns its status and raw data.
func (a *Anfra) Call(ctx context.Context, command string, args map[string]any) (string, json.RawMessage, error) {
	if args == nil {
		args = map[string]any{}
	}
	body, _ := json.Marshal(map[string]any{"command": command, "args": args})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://anfra/call", bytes.NewReader(body))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := a.http.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)

	var parsed struct {
		Status string          `json:"status"`
		Data   json.RawMessage `json:"data"`
		Error  string          `json:"error"`
	}
	_ = json.Unmarshal(raw, &parsed)
	if res.StatusCode != http.StatusOK {
		msg := parsed.Error
		if msg == "" {
			msg = fmt.Sprintf("anfra %s failed with %d", command, res.StatusCode)
		}
		return "", nil, &CallError{Message: msg, Status: res.StatusCode}
	}
	if parsed.Status == "" {
		parsed.Status = "ok"
	}
	return parsed.Status, parsed.Data, nil
}

// Stop terminates anfra and waits for it to exit.
func (a *Anfra) Stop() {
	if !a.Running() {
		return
	}
	_ = a.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-a.done:
	case <-time.After(5 * time.Second):
		_ = a.cmd.Process.Kill()
		<-a.done
	}
}

// IsAbort reports whether err comes from a cancelled context (a client that went away).
func IsAbort(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}
