package main

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/telemetry/stack"
	"github.com/holistics/anfra/shared/apperr"
	caller "github.com/holistics/anfra/shared/telemetry"
)

// runTelemetryInit runs `anfra telemetry init`, answering its result.
func runTelemetryInit(t *testing.T) (telemetryInitResult, error) {
	t.Helper()
	t.Cleanup(resetOutput)
	cmd := newRootCmd()
	cmd.SetArgs([]string{"telemetry", "init", "--yaml"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		return telemetryInitResult{}, err
	}
	var r telemetryInitResult
	if err := yaml.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	return r, nil
}

// inRepo runs the test in a new repo, with an ANFRA_HOME of its own and no
// endpoint set, and is the repo.
func inRepo(t *testing.T) repo.Local {
	t.Helper()
	t.Setenv("ANFRA_HOME", t.TempDir())
	for _, k := range []string{"OTEL_SDK_DISABLED", "OTEL_EXPORTER_OTLP_ENDPOINT", "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	dir := filepath.Join(t.TempDir(), "shop")
	if err := os.MkdirAll(filepath.Join(dir, repo.ConfigDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	return repo.Resolve(dir)
}

// init writes the repo's compose file into its state folder, named for the
// repo, and says what to do next, changing no setting: the endpoint is the
// user's to set, in the repo's .env.
func TestTelemetryInitWritesTheComposeFile(t *testing.T) {
	r := inRepo(t)
	res, err := runTelemetryInit(t)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(r.DataDir, "telemetry", "compose.yaml")
	if len(res.Files) != 1 || res.Files[0].Path != want || res.Files[0].Change != "created" {
		t.Fatalf("files %+v, want %s created", res.Files, want)
	}
	b, _ := os.ReadFile(want)
	if !strings.Contains(string(b), "name: "+stack.Name(r.ID)+"\n") {
		t.Errorf("the compose file isn't named for the repo:\n%s", b)
	}
	ports, _ := stack.PortsIn(want)
	hints := strings.Join(res.Hints, "\n")
	for _, s := range []string{"docker compose -f " + want + " up -d", "OTEL_EXPORTER_OTLP_ENDPOINT=" + ports.Endpoint(), ".env", "anfra status"} {
		if !strings.Contains(hints, s) {
			t.Errorf("hints lack %q:\n%s", s, hints)
		}
	}
	if _, err := os.Stat(filepath.Join(r.Dir, ".env")); !os.IsNotExist(err) {
		t.Errorf("init wrote the repo's .env: %v", err)
	}

	if res, err = runTelemetryInit(t); err != nil || res.Files[0].Change != "unchanged" {
		t.Errorf("again: %+v, %v; want unchanged", res.Files, err)
	}
}

// A second repo's stack takes other ports than the first's, even while the
// first is stopped, so both can run.
func TestTelemetryInitGivesEachRepoItsOwnPorts(t *testing.T) {
	first := inRepo(t)
	if _, err := runTelemetryInit(t); err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(t.TempDir(), "shop")
	if err := os.MkdirAll(filepath.Join(second, repo.ConfigDirName), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(second)
	res, err := runTelemetryInit(t)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := stack.PortsIn(filepath.Join(first.DataDir, "telemetry", "compose.yaml"))
	b, _ := stack.PortsIn(res.Files[0].Path)
	for _, p := range b.List() {
		if slices.Contains(a.List(), p) {
			t.Errorf("both stacks publish port %d: %+v, %+v", p, a, b)
		}
	}
}

// Outside a repo, init refuses: each repo has its own stack.
func TestTelemetryInitNeedsARepo(t *testing.T) {
	t.Setenv("ANFRA_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	_, err := runTelemetryInit(t)
	if got := apperr.From(err).Code; got != clierr.PreconditionFailed.Code() || !strings.Contains(err.Error(), "isn't an anfra repo") {
		t.Errorf("err %v (%s), want a precondition", err, got)
	}
}

// A compose.yaml anfra didn't write is refused, and left as it was.
func TestTelemetryInitLeavesSomeoneElsesFile(t *testing.T) {
	r := inRepo(t)
	dir := filepath.Join(r.DataDir, "telemetry")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	theirs := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(theirs, []byte("services: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := runTelemetryInit(t)
	if got := apperr.From(err).Code; got != clierr.PreconditionFailed.Code() || !strings.Contains(err.Error(), "didn't write") {
		t.Errorf("err %v (%s), want a precondition naming the file", err, got)
	}
	if b, _ := os.ReadFile(theirs); string(b) != "services: {}\n" {
		t.Errorf("their file changed: %q", b)
	}
}

// telemetry's help says what each data setting records, for every category
// the registry has.
func TestTelemetryHelpDescribesEveryDataSetting(t *testing.T) {
	help := traceSettingsHelp()
	for _, d := range caller.Categories {
		if categoryHelp[d.Name] == "" {
			t.Errorf("categoryHelp has no description of %s", d.Name)
		}
		if !strings.Contains(help, d.Var) {
			t.Errorf("the help doesn't list %s", d.Var)
		}
	}
}
