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
	AgentsMD *AgentsMDState `json:"agents_md" pretty:"AGENTS.md"`
}

// LatestSkills is the latest anfra skills version, from the background check,
// as `anfra version` names it; null until the check has run.
type LatestSkills struct {
	Latest    *string    `json:"latest"`
	CheckedAt *time.Time `json:"checked_at" pretty:",moment"`
}

// AgentState is where an agent's skills come from: the plugin, through its
// CLI, or copies in the folders it reads.
type AgentState struct {
	// ID is what --agent takes; not in the pretty format, where Name says
	// the same to a person, and --help lists the ids.
	ID   string `json:"id" pretty:"-"`
	Name string `json:"name"`
	// Status is installed, when it has a plugin or copies, or not-installed.
	Status  AgentStatus   `json:"status" pretty:",tone=installed:good not-installed:warn"`
	Plugins []PluginState `json:"plugins"`
	// Copies are the folders it reads that hold anfra skills, as Copies shows
	// them.
	Copies []string `json:"copies"`
	// InstallFrom is where to install the skills from: its README section;
	// absent when they're installed. Not in the pretty format, where a hint
	// says it, so the agents' table fits a terminal.
	InstallFrom string `json:"install_from,omitempty" pretty:"-"`
}

// AgentStatus is whether an agent has the anfra skills.
type AgentStatus string

const (
	AgentInstalled    AgentStatus = "installed"
	AgentNotInstalled AgentStatus = "not-installed"
)

// PluginState is an installed plugin against the latest skills.
type PluginState struct {
	Scope   string `json:"scope"`
	Version string `json:"version"`
	// Status is latest, behind, or unknown against an unknown latest.
	Status string `json:"status" pretty:",tone=latest:good behind:warn unknown:warn"`
}

// CopyState is a folder of copies of the skills against the latest.
type CopyState struct {
	Dir string `json:"dir"`
	// Status is latest, behind, mixed when its skills' versions differ, or
	// unknown against an unknown latest; deprecated when it has only skills of
	// the old names, which updating doesn't replace: they are removed.
	Status   string          `json:"status" pretty:",tone=latest:good behind:warn mixed:warn unknown:warn deprecated:bad"`
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
	Block BlockState `json:"block" pretty:",tone=up-to-date:good missing:warn outdated:warn unreadable:bad"`
	// ClaudeMD is whether AGENTS.md is imported in the repo's CLAUDE.md:
	// imported, not-imported (Claude Code misses the anfra block), or absent
	// when the repo has no CLAUDE.md, and Claude Code reads AGENTS.md on its
	// own.
	ClaudeMD ClaudeMDState `json:"claude_md" pretty:"CLAUDE.md,tone=imported:good not-imported:warn"`
	Message  string        `json:"message,omitempty"`
}

// ClaudeMDState is a repo's CLAUDE.md, against the import of AGENTS.md
// Claude Code needs to read it.
type ClaudeMDState string

const (
	ClaudeMDImported    ClaudeMDState = "imported"
	ClaudeMDNotImported ClaudeMDState = "not-imported"
	ClaudeMDAbsent      ClaudeMDState = "absent"
)

// BlockUnreadable: AGENTS.md or CLAUDE.md can't be read as a file of the repo.
const BlockUnreadable BlockState = "unreadable"

// CheckResult is `agent setup --check`'s result: the Report, between its
// message, the outcome, and its hints, what to do next
// (docs/designs/commands-and-api.md#cli-output).
type CheckResult struct {
	Message string `json:"message"`
	Report  `json:",inline"`
	// Hints are what to do next; absent when nothing is.
	Hints []string `json:"hints,omitempty"`
}

// SetupResult is setup's result: what it did, then the Report as it leaves
// things (docs/designs/commands-and-api.md#new-state-as-output), between its
// message and its hints.
type SetupResult struct {
	Message string       `json:"message"`
	Setup   SetupActions `json:"setup"`
	Report  `json:",inline"`
	// Hints are what to do next; absent when nothing is.
	Hints []string `json:"hints,omitempty"`
}

// SetupActions are what setup did.
type SetupActions struct {
	Agents []AgentSetup `json:"agents"`
	// AgentsMD is what setup wrote in the repo; null outside an anfra repo.
	AgentsMD *AgentsMDSetup `json:"agents_md" pretty:"AGENTS.md"`
}

// AgentSetup is what setup did for one agent.
type AgentSetup struct {
	// Agent is its id, what --agent takes; not in the pretty format, as the
	// report's agents.
	Agent   string  `json:"agent" pretty:"-"`
	Name    string  `json:"name"`
	Outcome Outcome `json:"outcome" pretty:",tone=installed:good failed:bad cli_missing:warn instructions_only:warn"`
	// Message is what to do next: restart the agent, install its CLI, or
	// follow the README.
	Message string `json:"message" pretty:",truncate"`
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
	res.Message, res.Hints = setupMessage(res.Setup), reportHints(res.Report, false)
	if failed {
		return res, ErrIncomplete
	}
	return res, nil
}

// install runs an agent's install steps, and says what came of it.
func install(ctx context.Context, env Env, a Agent, progress io.Writer) AgentSetup {
	s := AgentSetup{Agent: a.ID, Name: a.Name}
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
func Check(ctx context.Context, env Env, o Options) (CheckResult, error) {
	agents, _, err := selectAgents(env, o)
	if err != nil {
		return CheckResult{}, err
	}
	r := report(ctx, env, o, agents)
	return CheckResult{Message: checkMessage(r), Report: r, Hints: reportHints(r, true)}, nil
}

// report is the state of agents' skills, against the latest skills version
// from the background check:
//   - each agent, and where its skills come from: the plugin, through its CLI,
//     or copies in the folders it reads; or where to install them from;
//   - each copy found, the deprecated ones included, as an agent loads them
//     next to the new ones;
//   - in an anfra repo, its AGENTS.md block.
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
		s.Status = AgentInstalled
		if len(s.Plugins) == 0 && len(s.Copies) == 0 {
			s.Status, s.InstallFrom = AgentNotInstalled, a.README()
		}
		r.Agents = append(r.Agents, s)
	}

	for _, c := range copies {
		cs := CopyState{Dir: c.Dir, Versions: skillsVersions(c.Versions), DeprecatedNames: c.OldNames}
		if len(c.Versions) > 0 {
			cs.Status, _ = status(c.Versions, latest, known)
		} else {
			cs.Status = "deprecated"
		}
		r.Copies = append(r.Copies, cs)
	}

	if o.RepoDir != "" {
		r.AgentsMD = agentsMDState(o.RepoDir)
	}
	return r
}

// checkMessage is what r comes to, in a sentence: what needs attention, or
// that nothing does.
func checkMessage(r Report) string {
	var issues []string
	for _, a := range r.Agents {
		if a.Status == AgentNotInstalled {
			issues = append(issues, a.Name+" has no anfra skills")
		}
		for _, p := range a.Plugins {
			if p.Status == "behind" {
				issues = append(issues, fmt.Sprintf("%s's plugin is behind %s", a.Name, *r.Skills.Latest))
			}
		}
	}
	for _, c := range r.Copies {
		switch c.Status {
		case "behind":
			issues = append(issues, c.Dir+" is behind")
		case "mixed":
			issues = append(issues, c.Dir+" has skills at different versions")
		}
		if c.DeprecatedNames {
			issues = append(issues, c.Dir+" has deprecated skills")
		}
	}
	if md := r.AgentsMD; md != nil {
		switch {
		case md.Block == BlockMissing:
			issues = append(issues, "this repo's AGENTS.md has no anfra block")
		case md.Block == BlockOutdated:
			issues = append(issues, "this repo's AGENTS.md block is out of date")
		case md.Block == BlockUnreadable:
			issues = append(issues, "this repo's AGENTS.md can't be read")
		case md.ClaudeMD == ClaudeMDNotImported:
			issues = append(issues, "this repo's CLAUDE.md doesn't import AGENTS.md")
		}
	}
	if len(issues) == 0 {
		if r.Skills.Latest == nil {
			return "The anfra skills are set up in every agent. Their latest version isn't known yet."
		}
		return "The anfra skills are up to date in every agent."
	}
	return sentence(issues)
}

// setupMessage is what setup did, a sentence a kind of outcome, each agent by
// its name.
func setupMessage(s SetupActions) string {
	byOutcome := map[Outcome][]string{}
	for _, a := range s.Agents {
		byOutcome[a.Outcome] = append(byOutcome[a.Outcome], a.Name)
	}
	var parts []string
	if ids := byOutcome[OutcomeInstalled]; len(ids) > 0 {
		parts = append(parts, "Installed the anfra skills for "+joinAnd(ids)+".")
	}
	if ids := byOutcome[OutcomeFailed]; len(ids) > 0 {
		parts = append(parts, "Couldn't install them for "+joinAnd(ids)+".")
	}
	if ids := byOutcome[OutcomeCLIMissing]; len(ids) > 0 {
		parts = append(parts, "Didn't try "+joinAnd(ids)+", whose CLI isn't on your PATH.")
	}
	if ids := byOutcome[OutcomeInstructionsOnly]; len(ids) > 0 {
		verb := " needs"
		if len(ids) > 1 {
			verb = " need"
		}
		parts = append(parts, joinAnd(ids)+verb+" them installed by hand.")
	}
	if md := s.AgentsMD; md != nil {
		switch {
		case md.Message != "":
			parts = append(parts, "Couldn't write the anfra block in AGENTS.md.")
		case md.WroteBlock:
			parts = append(parts, "Wrote the anfra block in AGENTS.md.")
		}
	}
	return strings.Join(parts, " ")
}

// reportHints are what to do next about r: run setup (for a check), update
// or remove the copies setup doesn't touch, set up a repo.
func reportHints(r Report, check bool) []string {
	var hints, behind, deprecated, byHand []string
	setupWould := false
	for _, a := range r.Agents {
		if a.Status == AgentNotInstalled {
			if slices.ContainsFunc(Known, func(k Agent) bool { return k.ID == a.ID && k.Installs() }) {
				setupWould = true
			} else {
				byHand = append(byHand, fmt.Sprintf("Install the anfra skills for %s as %s says.", a.Name, a.InstallFrom))
			}
		}
		for _, p := range a.Plugins {
			setupWould = setupWould || p.Status == "behind"
		}
	}
	if md := r.AgentsMD; md != nil {
		setupWould = setupWould || md.Block == BlockMissing || md.Block == BlockOutdated || md.ClaudeMD == ClaudeMDNotImported
	}
	for _, c := range r.Copies {
		if c.Status == "behind" || c.Status == "mixed" {
			behind = append(behind, c.Dir)
		}
		if c.DeprecatedNames {
			deprecated = append(deprecated, c.Dir)
		}
	}
	if check && setupWould {
		hints = append(hints, "Run `anfra agent setup` to install or update them.")
	}
	hints = append(hints, byHand...)
	if len(behind) > 0 {
		hints = append(hints, fmt.Sprintf("To update the skills in %s, see %s#updating-and-removing.", joinAnd(behind), readmeURL))
	}
	if len(deprecated) > 0 {
		hints = append(hints, fmt.Sprintf("Remove the deprecated anfra skills in %s, from before %s, as %s#updating-and-removing says.",
			joinAnd(deprecated), renamedIn, readmeURL))
	}
	if r.AgentsMD == nil {
		skipped := "skipped"
		if check {
			skipped = "not checked"
		}
		hints = append(hints, "This folder isn't an anfra repo, so AGENTS.md was "+skipped+": run `anfra init` here to set it up.")
	}
	return hints
}

// sentence is issues as one sentence: capitalized, joined with commas and a
// final "and", and a full stop.
func sentence(issues []string) string {
	s := joinAnd(issues)
	return strings.ToUpper(s[:1]) + s[1:] + "."
}

// joinAnd joins items as prose: "a", "a and b", "a, b and c".
func joinAnd(items []string) string {
	if len(items) <= 1 {
		return strings.Join(items, "")
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
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
		return &AgentsMDState{Block: BlockUnreadable, ClaudeMD: ClaudeMDAbsent, Message: err.Error()}
	}
	s := &AgentsMDState{Block: b.State, ClaudeMD: ClaudeMDAbsent}
	switch {
	case b.NoImport:
		s.ClaudeMD = ClaudeMDNotImported
	case b.HasClaudeMD:
		s.ClaudeMD = ClaudeMDImported
	}
	return s
}
