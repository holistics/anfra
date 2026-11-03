package agents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/holistics/anfra/internal/update"
)

// Options are what setup acts on.
type Options struct {
	// Agents are the ids named with --agent; none: the agents detected.
	Agents []string
	// RepoDir is the anfra repo setup runs in, whose AGENTS.md gets the anfra
	// block, and whose copies of the skills the check looks for too; "" outside
	// one.
	RepoDir string
	// Progress is where to say, for a person, what setup is doing and what to
	// do next; nil says nothing. The result says what it did.
	Progress io.Writer
}

// Report is the state of the agents' skills: what `agent setup --check`
// answers, and setup with it, as setup leaves it.
type Report struct {
	Skills LatestSkills `json:"skills"`
	Agents []AgentState `json:"agents"`
	Copies []CopyState  `json:"copies"`
	// AgentsMD is the repo's anfra block; null outside an anfra repo.
	AgentsMD *AgentsMDState `json:"agents_md"`
}

// LatestSkills is the latest anfra skills version, from the background check,
// as `anfra version` names it; null until the check has run.
type LatestSkills struct {
	Latest    *string    `json:"latest"`
	CheckedAt *time.Time `json:"checked_at"`
}

// AgentState is where an agent's skills come from: the plugin, through its
// CLI, or copies in the folders it reads.
type AgentState struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Plugins []PluginState `json:"plugins"`
	// Copies are the folders it reads that hold anfra skills, as Copies shows
	// them.
	Copies []string `json:"copies"`
	// Install is where to install the skills from, when it has none.
	Install string `json:"install,omitempty"`
}

// PluginState is an installed plugin against the latest skills.
type PluginState struct {
	Scope   string `json:"scope"`
	Version string `json:"version"`
	// Status is latest, behind, or unknown against an unknown latest.
	Status string `json:"status"`
}

// CopyState is a folder of copies of the skills against the latest.
type CopyState struct {
	Dir string `json:"dir"`
	// Status is latest, behind, mixed when its skills' versions differ, or
	// unknown against an unknown latest; deprecated when it has only skills of
	// the old names, which updating doesn't replace: they are removed.
	Status   string          `json:"status"`
	Versions []SkillsVersion `json:"versions"`
	// DeprecatedNames: it has skills of the names from before renamedIn, which
	// an agent loads next to the new ones until they are removed.
	DeprecatedNames bool `json:"deprecated_names"`
}

// SkillsVersion is the skills of a copy at one version, oldest first.
type SkillsVersion struct {
	// Version is null for skills with no version stamp.
	Version *string  `json:"version"`
	Skills  []string `json:"skills"`
}

// AgentsMDState is the repo's anfra block in AGENTS.md.
type AgentsMDState struct {
	// Block is missing, outdated, up-to-date, or unreadable (Message says why).
	Block BlockState `json:"block"`
	// ClaudeMDImports is whether the repo's CLAUDE.md imports AGENTS.md; null
	// when it has none.
	ClaudeMDImports *bool  `json:"claude_md_imports"`
	Message         string `json:"message,omitempty"`
}

// BlockUnreadable: AGENTS.md or CLAUDE.md can't be read as a file of the repo.
const BlockUnreadable BlockState = "unreadable"

// SetupResult is setup's result: what it did, then the Report as it leaves
// things (docs/designs/commands-and-api.md#new-state-as-output).
type SetupResult struct {
	Setup  SetupActions `json:"setup"`
	Report `json:",inline"`
}

// SetupActions are what setup did.
type SetupActions struct {
	Agents []AgentSetup `json:"agents"`
	// AgentsMD is what setup wrote in the repo; null outside an anfra repo.
	AgentsMD *AgentsMDSetup `json:"agents_md"`
}

// AgentSetup is what setup did for one agent.
type AgentSetup struct {
	Agent   string  `json:"agent"`
	Outcome Outcome `json:"outcome"`
	// Message is what to do next: restart the agent, install its CLI, or
	// follow the README.
	Message string `json:"message"`
}

// Outcome is what setup did for an agent.
type Outcome string

const (
	OutcomeInstalled        Outcome = "installed"         // installed or updated
	OutcomeFailed           Outcome = "failed"            // an install step failed
	OutcomeCLIMissing       Outcome = "cli_missing"       // not tried: its CLI isn't on the PATH
	OutcomeInstructionsOnly Outcome = "instructions_only" // anfra doesn't install for it: the README says how
)

// AgentsMDSetup is what setup wrote of the anfra block.
type AgentsMDSetup struct {
	WroteBlock  bool `json:"wrote_block"`
	WroteImport bool `json:"wrote_import"`
	// Message is why it couldn't write them; absent when it could.
	Message string `json:"message,omitempty"`
}

// ErrIncomplete is setup leaving an agent without the skills, or the anfra
// block unwritten: Setup returns it with its result, as far as it got.
var ErrIncomplete = errors.New("the anfra skills are not set up for every agent")

// Setup installs the plugin into each agent anfra installs into, and writes
// the AGENTS.md block in an anfra repo, then reports the state it leaves, as
// Check does. It fails, with ErrIncomplete, when an install step failed or
// an agent named with --agent ends up without the plugin: an agent that was
// only detected doesn't fail it, as anyone with a second agent's folder would
// see setup fail on every run.
func Setup(ctx context.Context, env Env, o Options) (SetupResult, error) {
	agents, named, err := selectAgents(env, o)
	if err != nil {
		return SetupResult{}, err
	}
	res := SetupResult{Setup: SetupActions{Agents: []AgentSetup{}}}
	failed := false
	for _, a := range agents {
		s := install(ctx, env, a, o.Progress)
		res.Setup.Agents = append(res.Setup.Agents, s)
		failed = failed || s.Outcome == OutcomeFailed || named && s.Outcome != OutcomeInstalled
	}
	if o.RepoDir != "" {
		wroteBlock, wroteImport, err := WriteBlock(o.RepoDir)
		res.Setup.AgentsMD = &AgentsMDSetup{WroteBlock: wroteBlock, WroteImport: wroteImport}
		if err != nil {
			res.Setup.AgentsMD.Message = err.Error()
			failed = true
		}
	}
	// What setup installed, for the notice to know without asking the agents.
	RecordPlugins(ctx, env)

	res.Report = report(ctx, env, o, agents)
	if failed {
		return res, ErrIncomplete
	}
	return res, nil
}

// install runs an agent's install steps, and says what came of it.
func install(ctx context.Context, env Env, a Agent, progress io.Writer) AgentSetup {
	s := AgentSetup{Agent: a.ID}
	switch {
	case !a.Installs():
		s.Outcome, s.Message = OutcomeInstructionsOnly, "anfra doesn't install the skills for "+a.Name+": install them as "+a.README()+" says."
		return s
	case lookPathFails(env, a.plugin.bin):
		s.Outcome, s.Message = OutcomeCLIMissing, fmt.Sprintf("Its CLI `%s` is not on your PATH. Install %s, then run this again.", a.plugin.bin, a.Name)
		return s
	}
	if progress != nil {
		fmt.Fprintf(progress, "Installing %s for %s...\n", Plugin, a.Name)
	}
	for _, args := range a.plugin.install {
		if err := env.Run(ctx, a.plugin.bin, args...); err != nil {
			s.Outcome, s.Message = OutcomeFailed, fmt.Sprintf("`%s %s`: %v", a.plugin.bin, strings.Join(args, " "), err)
			return s
		}
	}
	s.Outcome, s.Message = OutcomeInstalled, a.plugin.note
	return s
}

func lookPathFails(env Env, bin string) bool {
	_, err := env.LookPath(bin)
	return err != nil
}

// selectAgents are the agents setup acts on: those named, else those detected.
// named says which. An id anfra doesn't know is still pointed at the README.
func selectAgents(env Env, o Options) (agents []Agent, named bool, err error) {
	if len(o.Agents) > 0 {
		for _, id := range o.Agents {
			a, ok := Find(id)
			if !ok {
				a = Agent{ID: id, Name: id}
			}
			agents = append(agents, a)
		}
		return agents, true, nil
	}
	for _, a := range Known {
		if a.Detected(env) {
			agents = append(agents, a)
		}
	}
	if len(agents) == 0 {
		return nil, false, fmt.Errorf("found no coding agent on this machine. Name yours with --agent, one of: %s.\n"+
			"For any other agent, see %s", strings.Join(IDs(), ", "), ReadmeFor(""))
	}
	return agents, false, nil
}

// status is skills' status against the latest: latest, behind, or mixed when
// their versions differ, and unknown against an unknown latest; with the
// details: their version, or which skills are at which version, oldest first,
// as "anfra-aql at 0.4.4; 7 skills at 0.4.5". "" is a skill with no stamp.
func status(versions map[string][]string, latest string, known bool) (string, string) {
	var vs []string
	for v := range versions {
		vs = append(vs, v)
	}
	slices.SortFunc(vs, func(a, b string) int {
		switch {
		case a == b:
			return 0
		case a == "" || Newer(b, a):
			return -1
		}
		return 1
	})
	label := func(v string) string {
		if v == "" {
			return "no version"
		}
		return v
	}
	if len(vs) > 1 {
		var parts []string
		for _, v := range vs {
			names := slices.Sorted(slices.Values(versions[v]))
			who := strings.Join(names, ", ")
			if len(names) > 3 {
				who = fmt.Sprintf("%d skills", len(names))
			}
			parts = append(parts, who+" at "+label(v))
		}
		return "mixed", strings.Join(parts, "; ")
	}
	v := vs[0]
	switch {
	case !known:
		return "unknown", label(v)
	case v == "" || Newer(latest, v):
		return "behind", label(v)
	}
	return "latest", v
}

// renamedIn is the skills release that gave their names the anfra- prefix:
// skills of the names from before it are deprecated.
const renamedIn = "0.15.0"

// Check reports the state of the agents' skills, as setup would find them,
// and changes nothing.
func Check(ctx context.Context, env Env, o Options) (Report, error) {
	agents, _, err := selectAgents(env, o)
	if err != nil {
		return Report{}, err
	}
	r := report(ctx, env, o, agents)
	if o.Progress != nil {
		fmt.Fprintln(o.Progress, "Run `anfra agent setup` to install or update them.")
	}
	return r, nil
}

// report is the state of agents' skills, against the latest skills version
// from the background check:
//   - each agent, and where its skills come from: the plugin, through its CLI,
//     or copies in the folders it reads; or where to install them from;
//   - each copy found, the deprecated ones included, as an agent loads them
//     next to the new ones;
//   - in an anfra repo, its AGENTS.md block.
//
// It tells progress what a person would do next: update copies, which setup
// doesn't, clean up deprecated ones, set up a repo.
func report(ctx context.Context, env Env, o Options, agents []Agent) Report {
	latest, checkedAt, known := update.CachedSkills()
	r := Report{Agents: []AgentState{}, Copies: []CopyState{}}
	if known {
		r.Skills = LatestSkills{Latest: &latest, CheckedAt: &checkedAt}
	}

	copies := Copies(env, o.RepoDir)
	for _, a := range agents {
		s := AgentState{ID: a.ID, Name: a.Name, Plugins: []PluginState{}, Copies: []string{}}
		for _, p := range a.InstalledPlugin(ctx, env) {
			st, _ := status(map[string][]string{p.Version: {Plugin}}, latest, known)
			s.Plugins = append(s.Plugins, PluginState{Scope: p.Scope, Version: p.Version, Status: st})
		}
		for _, c := range a.readsCopies(env, o.RepoDir, copies) {
			s.Copies = append(s.Copies, c.Dir)
		}
		if len(s.Plugins) == 0 && len(s.Copies) == 0 {
			s.Install = a.README()
		}
		r.Agents = append(r.Agents, s)
	}

	var behind, deprecated []string
	for _, c := range copies {
		cs := CopyState{Dir: c.Dir, Versions: skillsVersions(c.Versions), DeprecatedNames: c.OldNames}
		if len(c.Versions) > 0 {
			cs.Status, _ = status(c.Versions, latest, known)
		} else {
			cs.Status = "deprecated"
		}
		if cs.Status == "behind" || cs.Status == "mixed" {
			behind = append(behind, c.Dir)
		}
		if c.OldNames {
			deprecated = append(deprecated, c.Dir)
		}
		r.Copies = append(r.Copies, cs)
	}

	if o.RepoDir != "" {
		r.AgentsMD = agentsMDState(o.RepoDir)
	}

	if p := o.Progress; p != nil {
		if len(behind) > 0 {
			fmt.Fprintf(p, "To update the skills in %s, see %s#updating-and-removing.\n", strings.Join(behind, ", "), readmeURL)
		}
		if len(deprecated) > 0 {
			fmt.Fprintf(p, "%s has deprecated anfra skills, from before %s: clean them up as %s#updating-and-removing says.\n",
				strings.Join(deprecated, ", "), renamedIn, readmeURL)
		}
		if o.RepoDir == "" {
			fmt.Fprintln(p, "AGENTS.md: skipped, as this folder isn't an anfra repo (no .anfra/). Run `anfra init` here to set it up, with the anfra block.")
		}
	}
	return r
}

// skillsVersions is a copy's skills by version, oldest first, those with no
// stamp first of all, each version's skills sorted.
func skillsVersions(versions map[string][]string) []SkillsVersion {
	vs := make([]string, 0, len(versions))
	for v := range versions {
		vs = append(vs, v)
	}
	slices.SortFunc(vs, func(a, b string) int {
		switch {
		case a == b:
			return 0
		case a == "" || Newer(b, a):
			return -1
		}
		return 1
	})
	out := make([]SkillsVersion, 0, len(vs))
	for _, v := range vs {
		sv := SkillsVersion{Skills: slices.Sorted(slices.Values(versions[v]))}
		if v != "" {
			sv.Version = &v
		}
		out = append(out, sv)
	}
	return out
}

// agentsMDState is the anfra block in repoDir's AGENTS.md, and the import in
// its CLAUDE.md.
func agentsMDState(repoDir string) *AgentsMDState {
	b, err := CheckBlock(repoDir)
	if err != nil {
		return &AgentsMDState{Block: BlockUnreadable, Message: err.Error()}
	}
	s := &AgentsMDState{Block: b.State}
	if b.HasClaudeMD {
		imports := !b.NoImport
		s.ClaudeMDImports = &imports
	}
	return s
}
