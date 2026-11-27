package canallake

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/internal/sidecar/anfracanal"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
)

var ctx = context.Background()

// exitAtOnce makes this test program a canal-lake that exits as it starts.
const exitAtOnce = "CANALLAKE_TEST_EXIT_AT_ONCE"

func TestMain(m *testing.M) {
	if os.Getenv(exitAtOnce) != "" {
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// A canal-lake that exits as it starts fails Start at once, rather than after
// the wait for it to listen, and is tried again at the next Start.
func TestStartExited(t *testing.T) {
	// This test program, started again, as a fake canal-lake: a script can't
	// run on Windows.
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(exitAtOnce, "1")
	t.Setenv(anfracanal.Lake.BinEnv, bin)
	s, err := New(sidecar.Config{}, filepath.Join(t.TempDir(), "lake"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close(ctx)
	for range 2 {
		start := time.Now()
		if err := s.Start(ctx); err == nil || !strings.Contains(err.Error(), "exited") || time.Since(start) > 5*time.Second {
			t.Errorf("Start: %v after %s, want it to say canal-lake exited, at once", err, time.Since(start))
		}
	}
}

// canal-query writes a result to the canal-lake anfra started, as a Parquet
// file under the folder anfra gave it; and a canal-lake that exited is started
// again. It needs anfra-canal's binary, or both services', and is skipped
// without them.
func TestAgainstRealBinary(t *testing.T) {
	if os.Getenv(anfracanal.BinEnv) == "" && (os.Getenv(anfracanal.Lake.BinEnv) == "" || os.Getenv(anfracanal.Query.BinEnv) == "") {
		t.Skip("set ANFRA_CANAL_BIN to run against canal-lake")
	}
	storage := filepath.Join(t.TempDir(), "lake")
	cfg := sidecar.Config{StderrWriter: testWriter{t}}
	lake, err := New(cfg, storage)
	if err != nil {
		t.Fatal(err)
	}
	defer lake.Close(ctx)
	if err := lake.Start(ctx); err != nil {
		t.Fatal(err)
	}

	cfg.CanalLakeSocket = lake.SocketPath()
	query := canalquery.New(cfg)
	if err := query.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer query.Close(ctx)
	run := func(key string) {
		t.Helper()
		res, err := query.Client().ForTenant("t").Execute(ctx, canalquery.Statement{
			SQL: "SELECT 42 AS answer", DBType: "duckdb", DBConfig: map[string]any{}, TruncateRows: -1, LakeKey: key,
		})
		if err != nil || len(res.Rows) != 1 {
			t.Fatalf("query: %+v, %v", res, err)
		}
		if _, err := os.Stat(filepath.Join(storage, key+".parquet")); err != nil {
			t.Errorf("canal-lake's file: %v", err)
		}
	}
	run("1")

	// Exited, it is started again, at the same socket.
	lake.proc.Close(ctx)
	<-lake.proc.Done()
	if err := lake.Start(ctx); err != nil {
		t.Fatal(err)
	}
	run("2")

	lake.Close(ctx)
	if _, err := os.Stat(filepath.Dir(lake.SocketPath())); !os.IsNotExist(err) {
		t.Errorf("the socket's folder is left: %v", err)
	}
}

// testWriter is the sidecars' output, in the test's log.
type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}
