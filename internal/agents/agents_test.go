package agents

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/home"
)

// fakeEnv is a machine with the given CLIs on its PATH, an empty home, and the
// agents' plugin lists answering lists[bin]. It records what it runs.
type fakeEnv struct {
	Env
	ran   []string
	lists map[string]string
	fail  string // a command that fails, by its start ("claude plugin install")
}

func newFakeEnv(t *testing.T, onPath ...string) *fakeEnv {
	// anfra's own folder too: setup records what it installed there.
	t.Setenv("ANFRA_HOME", t.TempDir())
	f := &fakeEnv{lists: map[string]string{}}
	f.Env = Env{
		Home:   t.TempDir(),
		Getenv: func(string) string { return "" },
		LookPath: func(bin string) (string, error) {
			if slices.Contains(onPath, bin) {
				return "/bin/" + bin, nil
			}
			return "", exec.ErrNotFound
		},
		Run: func(_ context.Context, bin string, args ...string) error {
			cmd := bin + " " + strings.Join(args, " ")
			f.ran = append(f.ran, cmd)
			if f.fail != "" && strings.HasPrefix(cmd, f.fail) {
				return errors.New("exit status 1")
			}
			return nil
		},
		Output: func(_ context.Context, bin string, _ ...string) ([]byte, error) {
			if out, ok := f.lists[bin]; ok {
				return []byte(out), nil
			}
			return nil, errors.New("exit status 1")
		},
	}
	return f
}

const (
	claudeList = `[{"id":"gopls-lsp@claude-plugins-official","version":"1.0.0","scope":"user"},
		{"id":"anfra@anfra-skills","version":"0.4.5","scope":"user"}]`
	codexList = `{"installed":[{"pluginId":"anfra@anfra-skills","name":"anfra","marketplaceName":"anfra-skills","version":"0.4.4"}]}`
)

func TestSetupInstallsIntoEachAgentFound(t *testing.T) {
	env := newFakeEnv(t, "claude", "codex")
	env.lists["claude"] = claudeList
	var out bytes.Buffer
	if err := Setup(context.Background(), env.Env, Options{Out: &out}); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"claude plugin marketplace add holistics/anfra-skills --scope user",
		"claude plugin marketplace update anfra-skills",
		"claude plugin install anfra@anfra-skills --scope user",
		"claude plugin update anfra@anfra-skills --scope user",
		"codex plugin marketplace add holistics/anfra-skills",
		"codex plugin marketplace upgrade anfra-skills",
		"codex plugin add anfra@anfra-skills",
	}
	if !slices.Equal(env.ran, want) {
		t.Errorf("ran:\n%s\nwant:\n%s", strings.Join(env.ran, "\n"), strings.Join(want, "\n"))
	}
	for _, want := range []string{"Claude Code  plugin  plugin 0.4.5", "\nRestart Claude Code, or run /reload-plugins, to load it.\n"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the report lacks %q:\n%s", want, out.String())
		}
	}
}

// An agent anfra doesn't install into is pointed at its README section, and
// never reported as installed. Found, not named, it doesn't fail the run.
func TestSetupPointsOtherAgentsAtTheReadme(t *testing.T) {
	env := newFakeEnv(t, "opencode")
	var out bytes.Buffer
	if err := Setup(context.Background(), env.Env, Options{Out: &out}); err != nil {
		t.Fatal(err)
	}
	if len(env.ran) > 0 {
		t.Errorf("ran %v", env.ran)
	}
	report := out.String()
	want := "OpenCode  none  see https://github.com/holistics/anfra-skills#opencode"
	if !strings.Contains(report, want) {
		t.Errorf("report:\n%s\nwant a line:\n%s", report, want)
	}

	// Named, it fails the run: the agent asked for has no plugin.
	if err := Setup(context.Background(), env.Env, Options{Agents: []string{"opencode"}, Out: &bytes.Buffer{}}); err == nil {
		t.Error("a named agent left without the plugin: got no error")
	}
}

// An id anfra doesn't know is pointed at the README's section for any agent,
// which exists, rather than at a section that doesn't.
func TestSetupPointsAnUnknownAgentAtOtherAgents(t *testing.T) {
	env := newFakeEnv(t)
	var out bytes.Buffer
	_ = Setup(context.Background(), env.Env, Options{Agents: []string{"roo"}, Out: &out})
	if !strings.Contains(out.String(), "anfra-skills#other-agents") {
		t.Errorf("report:\n%s", out.String())
	}
}

func TestSetupWithNoAgentFound(t *testing.T) {
	env := newFakeEnv(t)
	err := Setup(context.Background(), env.Env, Options{Out: &bytes.Buffer{}})
	if err == nil || !strings.Contains(err.Error(), "claude-code") || !strings.Contains(err.Error(), "#other-agents") {
		t.Errorf("got %v, want an error listing the ids and the README", err)
	}
}

// An agent is found by its folder as well as its CLI. Found by its folder
// alone, it is reported as not installed, and doesn't fail the run; named, it
// does.
func TestSetupWithoutTheAgentsCLI(t *testing.T) {
	env := newFakeEnv(t)
	if err := os.MkdirAll(filepath.Join(env.Home, ".claude"), 0o750); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Setup(context.Background(), env.Env, Options{Out: &out}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Claude Code: not installed, as its CLI `claude` is not on your PATH.") {
		t.Errorf("report:\n%s", out.String())
	}
	if err := Setup(context.Background(), env.Env, Options{Agents: []string{"claude-code"}, Out: &out}); err == nil {
		t.Error("a named agent whose CLI is missing: got no error")
	}
}

// A folder named by an environment variable, as CODEX_HOME, finds the agent too.
func TestDetectedByAnEnvironmentVariable(t *testing.T) {
	env := newFakeEnv(t)
	codexHome := t.TempDir()
	env.Getenv = func(k string) string {
		if k == "CODEX_HOME" {
			return codexHome
		}
		return ""
	}
	codex, _ := Find("codex")
	if !codex.Detected(env.Env) {
		t.Error("Codex not found by CODEX_HOME")
	}
	claude, _ := Find("claude-code")
	if claude.Detected(env.Env) {
		t.Error("Claude Code found on an empty machine")
	}
}

// A failed step fails the run and is reported, and the other agents are still
// set up.
func TestSetupReportsAFailedStep(t *testing.T) {
	env := newFakeEnv(t, "claude", "codex")
	env.fail = "claude plugin install"
	var out bytes.Buffer
	if err := Setup(context.Background(), env.Env, Options{Out: &out}); err == nil {
		t.Error("a failed step: got no error")
	}
	if !strings.Contains(out.String(), "Claude Code: failed: `claude plugin install anfra@anfra-skills --scope user`") {
		t.Errorf("report:\n%s", out.String())
	}
	if !slices.Contains(env.ran, "codex plugin add anfra@anfra-skills") {
		t.Errorf("Codex wasn't set up after Claude Code failed: %v", env.ran)
	}
}

// Check reports where each agent's skills come from, each plugin and copy
// against the cached latest, and the deprecated copies, and changes nothing.
func TestCheckReports(t *testing.T) {
	env := newFakeEnv(t, "claude", "opencode", "codex")
	cacheSkills(t, "0.4.6")
	env.lists["claude"] = claudeList
	writeSkill(t, filepath.Join(env.Home, ".agents/skills"), "anfra-aql", stamped("0.4.6"))
	writeSkill(t, filepath.Join(env.Home, ".claude/skills"), "setup-repo", "Run `anfra version`.\n")
	var out bytes.Buffer
	if err := Check(context.Background(), env.Env, Options{NotRepo: true, Out: &out}); err != nil {
		t.Fatal(err)
	}
	want := `Latest anfra skills: 0.4.6, from GitHub (checked today)

Agents found:
  Claude Code  plugin, skills  plugin 0.4.5; ~/.claude/skills
  Codex        skills          ~/.agents/skills
  OpenCode     skills          ~/.agents/skills, ~/.claude/skills

Skills found:
  Claude Code plugin  behind  0.4.5
  ~/.agents/skills    latest  0.4.6

Deprecated skills, from before 0.15.0:
  ~/.claude/skills
Clean them up as https://github.com/holistics/anfra-skills#updating-and-removing says.

AGENTS.md:
  skipped: this folder isn't an anfra repo (no .anfra/). Run ` + "`anfra init`" + ` here to set it up, with the anfra block.

Run ` + "`anfra agent setup`" + ` to install or update them.
`
	if out.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", out.String(), want)
	}
	if len(env.ran) > 0 {
		t.Errorf("ran %v", env.ran)
	}
}

func TestPluginLists(t *testing.T) {
	if got := claudeInstalled([]byte(claudeList)); !slices.Equal(got, []Installed{{"0.4.5", "user"}}) {
		t.Errorf("claude: %v", got)
	}
	if got := codexInstalled([]byte(codexList)); !slices.Equal(got, []Installed{{"0.4.4", "user"}}) {
		t.Errorf("codex: %v", got)
	}
	if got := claudeInstalled([]byte("not json")); got != nil {
		t.Errorf("unreadable: %v", got)
	}
}

// cacheSkills puts latest in the background check's cache, as the latest
// skills version, checked now.
func cacheSkills(t *testing.T, latest string) {
	t.Helper()
	now := time.Now().Format(time.RFC3339)
	cache := fmt.Sprintf(`{"checked_at": %q, "latest_version": "9.0.0", "latest_tag": "v9.0.0", "skills_latest": %q, "skills_checked_at": %q}`, now, latest, now)
	if err := os.WriteFile(filepath.Join(home.Dir(), "update-check.json"), []byte(cache), 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeSkill writes the skill named name, with content, into the skills
// folder dir.
func writeSkill(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, name), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name, "SKILL.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// stamped is a skill's content ending with the stamp for version.
func stamped(version string) string {
	return "Run `anfra version`.\n\n---\nanfra skills " + version + "\n"
}

// The system Env runs only the agent CLIs anfra installs through, whatever
// program it is asked for; every plugin route's CLI is one of them.
func TestSystemRunsOnlyTheAgentCLIs(t *testing.T) {
	env := System()
	if err := env.Run(context.Background(), "sh", "-c", "echo hi"); err == nil || !strings.Contains(err.Error(), `runs no program "sh"`) {
		t.Errorf("Run sh: %v", err)
	}
	if _, err := env.Output(context.Background(), "/bin/echo", "hi"); err == nil {
		t.Error("Output /bin/echo: no error")
	}
	for _, a := range Known {
		if a.plugin == nil {
			continue
		}
		if _, err := agentCommand(context.Background(), a.plugin.bin, nil); err != nil {
			t.Errorf("%s's CLI %q isn't allowed: %v", a.Name, a.plugin.bin, err)
		}
	}
}

// In an anfra repo, setup writes the AGENTS.md block and reports it; a
// symlinked AGENTS.md fails the run.
func TestSetupWritesTheAgentsBlock(t *testing.T) {
	env := newFakeEnv(t, "opencode")
	repoDir := t.TempDir()
	var out bytes.Buffer
	if err := Setup(context.Background(), env.Env, Options{RepoDir: repoDir, Out: &out}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "AGENTS.md:\n  the anfra block is up-to-date (setup just wrote it)\n") {
		t.Errorf("report:\n%s", out.String())
	}
	out.Reset()
	if err := Setup(context.Background(), env.Env, Options{RepoDir: repoDir, Out: &out}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "AGENTS.md:\n  the anfra block is up-to-date\n") {
		t.Errorf("report:\n%s", out.String())
	}

	linked := t.TempDir()
	if err := os.Symlink(filepath.Join(t.TempDir(), "x.md"), filepath.Join(linked, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := Setup(context.Background(), env.Env, Options{RepoDir: linked, Out: &out}); err == nil {
		t.Error("a symlinked AGENTS.md: got no error")
	}
	if !strings.Contains(out.String(), "AGENTS.md: failed: AGENTS.md is not a regular file") {
		t.Errorf("report:\n%s", out.String())
	}
}

// In an anfra repo, check reports the block, and the import CLAUDE.md lacks.
func TestCheckReportsTheAgentsBlock(t *testing.T) {
	env := newFakeEnv(t, "opencode")
	repoDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoDir, "CLAUDE.md"), []byte("# Notes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Check(context.Background(), env.Env, Options{RepoDir: repoDir, Out: &out}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\nAGENTS.md:\n  the anfra block is missing\n  CLAUDE.md doesn't import it (@AGENTS.md)\n") {
		t.Errorf("report:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(repoDir, "AGENTS.md")); err == nil {
		t.Error("check wrote AGENTS.md")
	}
}

// With nothing installed, the report says how to install them.
func TestCheckWithNoSkills(t *testing.T) {
	env := newFakeEnv(t, "opencode")
	var out bytes.Buffer
	if err := Check(context.Background(), env.Env, Options{Out: &out}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Skills found:\n  none. Run `anfra agent setup` to install them in your agents.\n") {
		t.Errorf("report:\n%s", out.String())
	}
}
