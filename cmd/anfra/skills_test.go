package main

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/app"
)

// fakeSkillsEnv has the given CLIs on its PATH and records what it runs.
func fakeSkillsEnv(onPath ...string) (skillsEnv, *[]string, *bytes.Buffer) {
	var ran []string
	var out bytes.Buffer
	return skillsEnv{
		lookPath: func(bin string) (string, error) {
			if slices.Contains(onPath, bin) {
				return "/bin/" + bin, nil
			}
			return "", exec.ErrNotFound
		},
		run: func(_ context.Context, bin string, args ...string) error {
			ran = append(ran, bin+" "+strings.Join(args, " "))
			return nil
		},
		out: &out,
	}, &ran, &out
}

func TestSkillsInstallIntoEachAgentOnThePath(t *testing.T) {
	env, ran, _ := fakeSkillsEnv("claude", "codex")
	if err := installSkills(context.Background(), env, nil, scopeUser); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"claude plugin marketplace add holistics/anfra-skills --scope user",
		"claude plugin marketplace update anfra-skills",
		"claude plugin install anfra@anfra-skills --scope user",
		"codex plugin marketplace add holistics/anfra-skills",
		"codex plugin marketplace upgrade anfra-skills",
		"codex plugin add anfra@anfra-skills",
	}
	if !slices.Equal(*ran, want) {
		t.Errorf("ran:\n%s\nwant:\n%s", strings.Join(*ran, "\n"), strings.Join(want, "\n"))
	}
}

func TestSkillsInstallWithNoAgentOnThePath(t *testing.T) {
	env, ran, _ := fakeSkillsEnv()
	err := installSkills(context.Background(), env, nil, scopeUser)
	if err == nil || !strings.Contains(err.Error(), "npx skills add holistics/anfra-skills") {
		t.Errorf("got %v, want an error naming the other ways to install", err)
	}
	if len(*ran) > 0 {
		t.Errorf("ran %v", *ran)
	}
}

func TestSkillsInstallIntoANamedAgent(t *testing.T) {
	env, ran, _ := fakeSkillsEnv("claude", "codex")
	if err := installSkills(context.Background(), env, []string{"codex"}, scopeUser); err != nil {
		t.Fatal(err)
	}
	for _, r := range *ran {
		if !strings.HasPrefix(r, "codex ") {
			t.Errorf("ran %q for an agent not named", r)
		}
	}

	env, _, _ = fakeSkillsEnv()
	if err := installSkills(context.Background(), env, []string{"claude"}, scopeUser); err == nil {
		t.Error("a named agent whose CLI is missing: got no error")
	}
	if err := installSkills(context.Background(), env, []string{"gemini"}, scopeUser); err == nil {
		t.Error("an unknown agent: got no error")
	}
}

// Cursor's CLI cannot install a plugin, so its steps are shown, not run.
func TestSkillsInstallForCursorShowsTheSteps(t *testing.T) {
	env, ran, out := fakeSkillsEnv("claude", "codex")
	if err := installSkills(context.Background(), env, []string{"cursor"}, scopeUser); err != nil {
		t.Fatal(err)
	}
	if len(*ran) > 0 {
		t.Errorf("ran %v", *ran)
	}
	if !strings.Contains(out.String(), "Team Marketplaces") {
		t.Errorf("output %q does not show Cursor's steps", out.String())
	}
}

func TestSkillsInstallIntoTheProject(t *testing.T) {
	env, ran, out := fakeSkillsEnv("claude", "codex")
	if err := installSkills(context.Background(), env, nil, scopeProject); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"claude plugin marketplace add holistics/anfra-skills --scope project",
		"claude plugin marketplace update anfra-skills",
		"claude plugin install anfra@anfra-skills --scope project",
	}
	if !slices.Equal(*ran, want) {
		t.Errorf("ran:\n%s\nwant:\n%s", strings.Join(*ran, "\n"), strings.Join(want, "\n"))
	}
	if !strings.Contains(out.String(), "Skipped Codex") {
		t.Errorf("output %q does not say Codex was skipped", out.String())
	}

	// Named, Codex cannot be installed into the project, so nothing is.
	env, ran, _ = fakeSkillsEnv("claude", "codex")
	if err := installSkills(context.Background(), env, []string{"claude", "codex"}, scopeProject); err == nil {
		t.Error("codex named with --scope project: got no error")
	}
	if len(*ran) > 0 {
		t.Errorf("ran %v", *ran)
	}

	if err := installSkills(context.Background(), env, nil, "global"); err == nil {
		t.Error("an unknown scope: got no error")
	}
}

func TestSkillsInstallStopsAtAFailedStep(t *testing.T) {
	env, ran, _ := fakeSkillsEnv("claude", "codex")
	env.run = func(_ context.Context, bin string, args ...string) error {
		*ran = append(*ran, bin+" "+strings.Join(args, " "))
		return errors.New("exit status 1")
	}
	err := installSkills(context.Background(), env, nil, scopeUser)
	if err == nil || !strings.Contains(err.Error(), "claude plugin marketplace add") {
		t.Errorf("got %v, want an error naming the failed command", err)
	}
	if len(*ran) != 1 {
		t.Errorf("ran %v after the failure", *ran)
	}
}

// Installing runs programs on this machine, so it stays a CLI command and is
// never registered as an op that a server would serve.
func TestSkillsInstallIsNotAnOp(t *testing.T) {
	for _, name := range []string{"skills", "skills.install"} {
		if _, ok := app.Find(name); ok {
			t.Errorf("%s is a registered command, so it is served as an op", name)
		}
	}
}
