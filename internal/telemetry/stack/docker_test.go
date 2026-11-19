package stack_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"

	"github.com/holistics/anfra/internal/telemetry"
	"github.com/holistics/anfra/internal/telemetry/stack"
	caller "github.com/holistics/anfra/shared/telemetry"
)

// The telemetry stack, for real: the file Write writes starts on Docker, as the
// usage page says to start it, and a span anfra exports to it lands in
// otel_traces. It starts containers, so it runs only when asked: ANFRA_TEST_TELEMETRY_STACK=1, as CI's telemetry-stack job does.
func TestTheStackRecordsASpan(t *testing.T) {
	if os.Getenv("ANFRA_TEST_TELEMETRY_STACK") != "1" {
		t.Skip("set ANFRA_TEST_TELEMETRY_STACK=1 to start the telemetry stack on Docker")
	}
	ctx := context.Background()
	// A stack of the test's own, on free ports, as a repo's stack is; one by
	// that name already on this machine is refused, as `down --volumes` would
	// delete it.
	name := stack.Name("anfra-stack-check")
	out, err := exec.CommandContext(ctx, "docker", "compose", "--project-name", name, "ps", "--all", "--quiet").Output() //nolint:gosec // G204: fixed arguments
	if err != nil || len(bytes.TrimSpace(out)) > 0 {
		t.Fatalf("a %s stack is on this machine already (%v), or Docker isn't running: stop and remove it first, as this test deletes its own", name, err)
	}
	w, err := stack.Write(t.TempDir(), stack.Spec{Version: "test", Name: name, Taken: stack.DefaultPorts.List()})
	if err != nil {
		t.Fatal(err)
	}
	file, ports := w.Path, w.Ports
	t.Cleanup(func() {
		if err := compose(context.Background(), file, "down", "--volumes"); err != nil {
			t.Errorf("down: %v", err)
		}
	})
	if err := compose(ctx, file, "up", "--detach", "--wait"); err != nil {
		t.Fatal(err)
	}
	if err := waitReady(ctx, ports.OTLPHTTP); err != nil {
		t.Fatal(err)
	}

	for _, k := range []string{"OTEL_SDK_DISABLED", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "H_OTEL_ENABLED", "H_OTEL_INTERNAL_EXPORTER_URL", "H_OTEL_TRACES_EXPORTER"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	t.Setenv("ANFRA_HOME", t.TempDir())
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", ports.Endpoint())
	shutdown, err := telemetry.Setup(ctx, "anfra", "test", caller.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	spanName := fmt.Sprintf("anfra stack-check %d", time.Now().UnixNano())
	_, span := otel.Tracer("test").Start(ctx, spanName)
	span.End()
	if err := shutdown(ctx); err != nil {
		t.Fatal(err)
	}

	query := fmt.Sprintf("SELECT count() FROM %s.otel_traces WHERE SpanName = '%s'", stack.Database, spanName)
	deadline := time.Now().Add(30 * time.Second)
	for {
		got, err := clickhouse(ctx, ports.ClickHouseHTTP, query)
		if err == nil && got == "1" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the span never reached otel_traces: got %q, %v", got, err)
		}
		time.Sleep(time.Second)
	}
}

// waitReady waits until the collector on port takes spans: an empty OTLP/HTTP
// export answered 200. Its container is healthy a moment before it does.
func waitReady(ctx context.Context, port int) error {
	u := fmt.Sprintf("http://127.0.0.1:%d/v1/traces", port)
	deadline := time.Now().Add(60 * time.Second)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader("{}"))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the collector did not take spans within a minute")
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// compose runs docker compose on the file, as a user would.
func compose(ctx context.Context, file string, args ...string) error {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose", "--file", file}, args...)...) //nolint:gosec // G204: the test's own file and arguments
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose %s: %w\n%s", strings.Join(args, " "), err, out.String())
	}
	return nil
}

// clickhouse is query's answer from the stack's ClickHouse, trimmed.
func clickhouse(ctx context.Context, port int, query string) (string, error) {
	u := fmt.Sprintf("http://localhost:%d/?user=%s&password=%s", port, url.QueryEscape(stack.User), url.QueryEscape(stack.Password))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(query))
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: %s", resp.Status, b)
	}
	return strings.TrimSpace(string(b)), err
}
