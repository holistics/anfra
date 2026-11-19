package repoenv

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canallake"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
)

// repoWith is a repo (a folder with .anfra/) whose .env is content.
func repoWith(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".anfra"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// unset unsets names for the test, restoring them after.
func unset(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		t.Setenv(n, "")
		_ = os.Unsetenv(n)
	}
}

// Every name of anfra's in the file is set, unless the environment sets it
// already; the last line of a name given twice wins; and each says where it
// came from.
func TestLoadSetsWhatTheEnvironmentDoesnt(t *testing.T) {
	unset(t, "OTEL_EXPORTER_OTLP_ENDPOINT", "ANFRA_TRACE_INPUTS")
	t.Setenv("ANFRA_LOG_STDERR", "0")
	dir := repoWith(t, "# telemetry\n"+
		"OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318\n"+
		"export ANFRA_TRACE_INPUTS=off\n"+
		"ANFRA_TRACE_INPUTS=raw\n"+
		"ANFRA_LOG_STDERR=1\n")
	l, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "ANFRA_TRACE_INPUTS"}; !slices.Equal(l.Set, want) {
		t.Errorf("Set = %v, want %v", l.Set, want)
	}
	for name, want := range map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://localhost:4318",
		"ANFRA_TRACE_INPUTS":          "raw",
		"ANFRA_LOG_STDERR":            "0",
	} {
		if got := os.Getenv(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	if got := Describe("OTEL_EXPORTER_OTLP_ENDPOINT"); got != "OTEL_EXPORTER_OTLP_ENDPOINT in .env" {
		t.Errorf("Describe = %q", got)
	}
	if line, ok := Origin("ANFRA_TRACE_INPUTS"); !ok || line != "line 4" {
		t.Errorf("Origin = %q, %v; want line 4", line, ok)
	}
	if got := Describe("ANFRA_LOG_STDERR"); got != "ANFRA_LOG_STDERR" {
		t.Errorf("the environment's: Describe = %q", got)
	}
	// Set again by someone else, it is no longer the file's.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://elsewhere:4318")
	if got := Describe("OTEL_EXPORTER_OTLP_ENDPOINT"); got != "OTEL_EXPORTER_OTLP_ENDPOINT" {
		t.Errorf("after the environment changed: Describe = %q", got)
	}
}

// Only anfra's names are read: another tool's, such as an app's credentials or
// PATH, and a sidecar's prefixed name, which sets any name in it, reach neither
// anfra nor a sidecar, which inherits anfra's environment.
func TestLoadReadsOnlyAnfrasNames(t *testing.T) {
	others := []string{"PGPASSWORD", "PATH_EXTRA", "ANFRA_NODE_ENV_NODE_OPTIONS", "ANFRA_CANAL_QUERY_ENV_OTEL_EXPORTER_OTLP_ENDPOINT", "ANFRA_CANAL_LAKE_ENV_GOMEMLIMIT"}
	unset(t, append([]string{"ANFRA_TRACE_SQL"}, others...)...)
	dir := repoWith(t, "ANFRA_TRACE_SQL=raw\n"+
		"PGPASSWORD=secret\n"+
		"PATH_EXTRA=/tmp/evil\n"+
		"ANFRA_NODE_ENV_NODE_OPTIONS=--require=/tmp/evil.js\n"+
		"ANFRA_CANAL_QUERY_ENV_OTEL_EXPORTER_OTLP_ENDPOINT=http://elsewhere:4318\n"+
		"ANFRA_CANAL_LAKE_ENV_GOMEMLIMIT=1KiB\n")
	l, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"ANFRA_TRACE_SQL"}; !slices.Equal(l.Set, want) {
		t.Errorf("Set = %v, want %v", l.Set, want)
	}
	for _, n := range others {
		if v, ok := os.LookupEnv(n); ok {
			t.Errorf("%s = %q, want it unset", n, v)
		}
	}
}

// The sidecar prefixes loaded skips are the sidecars'.
func TestSidecarPrefixesAreTheSidecars(t *testing.T) {
	for _, p := range []string{anfranode.EnvPrefix, canalquery.EnvPrefix, canallake.EnvPrefix} {
		if !slices.Contains(sidecarPrefixes, p) {
			t.Errorf("sidecarPrefixes = %v, missing %s", sidecarPrefixes, p)
		}
	}
}

// A folder that isn't a repo is never read, whatever its .env holds.
func TestLoadReadsOnlyARepo(t *testing.T) {
	unset(t, "OTEL_EXPORTER_OTLP_ENDPOINT")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte("OTEL_EXPORTER_OTLP_ENDPOINT=http://x:4318\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if l, err := Load(dir); err != nil || l.Path != "" || os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		t.Errorf("Load = %+v, %v; want nothing read", l, err)
	}
}

// A .env that Git tracks is never read: a clone's author must not choose where
// its traces go. Its anfra and OpenTelemetry names are listed for a warning.
func TestATrackedFileIsNotRead(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	unset(t, "OTEL_EXPORTER_OTLP_ENDPOINT", "ANFRA_TRACE_INPUTS", "NODE_ENV")
	dir := repoWith(t, "OTEL_EXPORTER_OTLP_ENDPOINT=http://attacker:4318\nANFRA_TRACE_INPUTS=raw\nNODE_ENV=dev\n")
	for _, args := range [][]string{{"init", "-q"}, {"add", FileName}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil { //nolint:gosec // G204: the test's own folder and arguments
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	l, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !l.Tracked || len(l.Set) > 0 || !slices.Equal(l.Ignored, []string{"OTEL_EXPORTER_OTLP_ENDPOINT", "ANFRA_TRACE_INPUTS"}) {
		t.Errorf("Load = %+v", l)
	}
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" || os.Getenv("NODE_ENV") != "" {
		t.Error("a tracked file was read")
	}
}

// A line of anfra's that isn't NAME=value is refused with its line number,
// and nothing in the file is set.
func TestABadLineOfAnfrasIsRefused(t *testing.T) {
	for name, content := range map[string]string{
		"no =":           "ANFRA_TRACE_SQL=raw\nOTEL_EXPORTER_OTLP_ENDPOINT http://x:4318\n",
		"unclosed quote": "ANFRA_TRACE_SQL=raw\nANFRA_TRACE_INPUTS=\"open\n",
		"after quote":    "ANFRA_TRACE_SQL=raw\nANFRA_TRACE_INPUTS='x' y\n",
	} {
		t.Run(name, func(t *testing.T) {
			unset(t, "ANFRA_TRACE_SQL", "ANFRA_TRACE_INPUTS", "OTEL_EXPORTER_OTLP_ENDPOINT")
			_, err := Load(repoWith(t, content))
			if err == nil || !strings.Contains(err.Error(), ".env") || !strings.Contains(err.Error(), "line 2") {
				t.Errorf("err = %v, want one naming .env line 2", err)
			}
			if os.Getenv("ANFRA_TRACE_SQL") != "" {
				t.Error("a file with a bad line was partly read")
			}
		})
	}
}

// The file may be an app's, in its own tool's dialect: a line anfra can't
// read that isn't anfra's is skipped, and anfra's names are still read.
func TestAnotherToolsLinesAreSkipped(t *testing.T) {
	unset(t, "ANFRA_TRACE_SQL", "OTEL_EXPORTER_OTLP_ENDPOINT", "PRIVATE_KEY", "B")
	dir := repoWith(t, "spring.datasource.url=jdbc:postgresql://db/app\n"+
		"PRIVATE_KEY=\"-----BEGIN KEY-----\nOTEL_EXPORTER_OTLP_ENDPOINT=http://inside-the-key\n-----END KEY-----\"\n"+
		"B='x' y\n"+
		"not a setting\n"+
		"ANFRA_TRACE_SQL=raw\n")
	l, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"ANFRA_TRACE_SQL"}; !slices.Equal(l.Set, want) {
		t.Errorf("Set = %v, want %v", l.Set, want)
	}
	if v := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); v != "" {
		t.Errorf("a line inside a multi-line value was read: OTEL_EXPORTER_OTLP_ENDPOINT = %q", v)
	}
}

// A .env Git tracks that holds none of anfra's names isn't anfra's business:
// Git isn't asked, and nothing is warned of.
func TestAFileWithoutAnfrasNamesIsNotChecked(t *testing.T) {
	unset(t, "NODE_ENV")
	dir := repoWith(t, "NODE_ENV=dev\n")
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir()) // no git: asking would leave it unchecked
	l, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if l.Unchecked || l.Tracked || len(l.Ignored) > 0 || len(l.Set) > 0 {
		t.Errorf("Load = %+v, want nothing checked or set", l)
	}
}

// Environ leaves out what the file set, so another anfra started in the repo
// reads the file itself and knows where its settings came from.
func TestEnvironLeavesOutTheFile(t *testing.T) {
	unset(t, "ANFRA_TRACE_SQL")
	t.Setenv("ANFRA_TRACE_INPUTS", "masked")
	if _, err := Load(repoWith(t, "ANFRA_TRACE_SQL=raw\nANFRA_TRACE_INPUTS=raw\n")); err != nil {
		t.Fatal(err)
	}
	env := Environ()
	if slices.Contains(env, "ANFRA_TRACE_SQL=raw") {
		t.Error("Environ kept the file's ANFRA_TRACE_SQL")
	}
	if !slices.Contains(env, "ANFRA_TRACE_INPUTS=masked") {
		t.Error("Environ dropped the environment's ANFRA_TRACE_INPUTS")
	}
}

func TestParse(t *testing.T) {
	got, bad := Parse([]byte("\uFEFFA=1\n\n  # c\nB = two words # note\nC=\"#not a comment\"\nD=\nE=a=b\r\n" +
		"F=\"a \\\"q\\\" \\\\ b\"\nG='two\nlines'\nH='open\nI=after\n"))
	want := []Entry{{"A", "1", 1}, {"B", "two words", 4}, {"C", "#not a comment", 5}, {"D", "", 6}, {"E", "a=b", 7},
		{"F", `a "q" \ b`, 8}, {"G", "two\nlines", 9}, {"I", "after", 12}}
	if !slices.Equal(got, want) {
		t.Errorf("Parse = %+v\nwant %+v", got, want)
	}
	if len(bad) != 1 || bad[0].Line != 11 || bad[0].Name != "H" {
		t.Errorf("bad = %+v, want H's unclosed quote on line 11", bad)
	}
}

// In a Git work tree where Git can't say whether it tracks the file (no git,
// as in anfra's Docker image), the file isn't read either.
func TestAFileGitCantCheckIsNotRead(t *testing.T) {
	unset(t, "OTEL_EXPORTER_OTLP_ENDPOINT")
	dir := repoWith(t, "OTEL_EXPORTER_OTLP_ENDPOINT=http://attacker:4318\n")
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir()) // no git on it
	l, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !l.Unchecked || l.Tracked || len(l.Set) > 0 || os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" {
		t.Errorf("Load = %+v, want nothing read", l)
	}
}
