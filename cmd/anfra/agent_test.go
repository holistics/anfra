package main

import (
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/app"
)

// Setting up agents runs programs on this machine, so it stays a CLI command
// and is never registered as an op that a server would serve.
func TestAgentSetupIsNotAnOp(t *testing.T) {
	for _, name := range []string{"agent", "agent.setup", "skills", "skills.install"} {
		if _, ok := app.Find(name); ok {
			t.Errorf("%s is a registered command, so it is served as an op", name)
		}
	}
}

// The former command's --scope project, which installed into the repo's Claude
// Code settings, is refused with what to run instead.
func TestSkillsInstallRefusesTheProjectScope(t *testing.T) {
	cmd := newSkillsCmd()
	cmd.SetArgs([]string{"install", "--scope", "project"})
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--scope project is no longer supported: use `anfra agent setup`") {
		t.Errorf("got %v", err)
	}
}
