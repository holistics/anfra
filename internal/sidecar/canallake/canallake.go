// Package canallake is the canal-lake sidecar, which stores the query cache's
// results as Parquet files: its supervisor. anfra never calls canal-lake
// itself. canal-query writes to it, reads from it and drops its files, at the
// socket anfra gives both (sidecar.Config.CanalLakeSocket). So the supervisor
// has no client: it starts canal-lake (anfra-canal's lake service, see
// anfracanal) when the cache first needs it, and again if it has exited.
// See docs/designs/query-cache.md#canal-lake-as-a-sidecar.
package canallake

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/internal/sidecar/anfracanal"
	"github.com/holistics/anfra/shared/logkit"
)

// Name is the sidecar's name, in its logs and its errors.
const Name = "canal-lake"

// EnvPrefix sets canal-lake's own settings from anfra's environment:
// ANFRA_CANAL_LAKE_ENV_PPROF_ADDR=localhost:6062 is its PPROF_ADDR
// (sidecar.ProcSpec.EnvPrefix).
const EnvPrefix = "ANFRA_CANAL_LAKE_ENV_"

// shutdownGrace is how long canal-lake waits for its calls when stopped.
// Its default is 20 minutes; anfra kills a sidecar after 3 seconds anyway
// (sidecar.Process.Close).
const shutdownGrace = "2"

// Sidecar supervises one canal-lake, spawned at its socket on first Start.
type Sidecar struct {
	cfg        sidecar.Config
	storage    string
	socketPath string

	mu   sync.Mutex
	proc *sidecar.Process
}

// New is canal-lake's supervisor, its storage in the folder storage, not yet
// started. Its socket's path is chosen now, so canal-query can be given it
// before canal-lake runs (SocketPath).
func New(cfg sidecar.Config, storage string) (*Sidecar, error) {
	socketPath, err := sidecar.SocketPath(Name)
	if err != nil {
		return nil, err
	}
	return &Sidecar{cfg: cfg, storage: storage, socketPath: socketPath}, nil
}

// SocketPath is where canal-lake listens, once started.
func (s *Sidecar) SocketPath() string { return s.socketPath }

// Start starts canal-lake unless it runs already, and waits until it listens.
// One that exited, or failed to start, is started again.
func (s *Sidecar) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proc != nil {
		select {
		case <-s.proc.Done():
			s.proc = nil // exited: start it again
		default:
			return nil
		}
	}

	cmd, err := anfracanal.Lake.Command()
	if err != nil {
		return fmt.Errorf("resolve canal-lake binary: %w", err)
	}
	// canal-lake appends duckdb.tmp/ to STORAGE_ROOT for DuckDB's temporary
	// files, so it must end in a separator.
	if err := os.MkdirAll(s.storage, 0o700); err != nil {
		return fmt.Errorf("create canal-lake's storage folder: %w", err)
	}
	_ = os.Remove(s.socketPath) // one an earlier canal-lake left
	proc, err := sidecar.StartProcess(ctx, sidecar.ProcSpec{
		Name: Name,
		Path: cmd.Path,
		Args: cmd.Args,
		// It loads a .env from the folder it runs in: its own, not the repo's.
		Dir: s.storage,
		ExtraEnv: []string{
			"SOCKET=" + s.socketPath,
			"STORAGE_ROOT=" + s.storage + string(os.PathSeparator),
			"SHUTDOWN_GRACE_SECONDS=" + shutdownGrace,
		},
		EnvPrefix: EnvPrefix,
		Stdout:    s.cfg.StdoutWriter,
		Stderr:    s.cfg.StderrWriter,
		Logger:    s.cfg.Logger,
	})
	if err != nil {
		return err
	}
	if err := waitListening(ctx, s.socketPath, proc.Done()); err != nil {
		proc.Close(ctx)
		return err
	}
	s.proc = proc
	proc.Logger().InfoContext(ctx, "Sidecar ready", logkit.EventKey, sidecar.EventReady, "name", Name, "socket", s.socketPath)
	return nil
}

// readyWait is how long Start waits for canal-lake to listen; a variable so
// that a test can wait less.
var readyWait = 15 * time.Second

// waitListening dials socket until it accepts a connection. canal-lake opens
// it once its DuckDB pool is up, so a connection accepted is a canal-lake
// ready for calls. It gives up when exited closes: canal-lake failed to
// start, and its output, in the log, says why.
func waitListening(ctx context.Context, socket string, exited <-chan struct{}) error {
	deadline := time.Now().Add(readyWait)
	for {
		var d net.Dialer
		conn, err := d.DialContext(ctx, "unix", socket)
		if err == nil {
			conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("canal-lake didn't start within %s", readyWait)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("canal-lake not ready: %w", ctx.Err())
		case <-exited:
			return errors.New("canal-lake exited as it started")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Close stops canal-lake, if it runs, and removes its socket.
//
// ctx carries the trace its log record joins; it is not for cancellation
// (sidecar.Process.Close).
func (s *Sidecar) Close(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.proc != nil {
		s.proc.Close(ctx)
		s.proc = nil
	}
	sidecar.RemoveSocket(s.socketPath)
}
