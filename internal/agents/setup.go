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
	Out     io.Writer
}

// Setup installs the plugin into each agent anfra installs into, and writes
// the AGENTS.md block in an anfra repo, then reports the state it leaves, as
// Check does, and what to do next. It fails when an install step failed or an
// agent named with --agent ends up without the plugin: an agent that was only
// detected doesn't fail it, as anyone with a second agent's folder would see
// setup fail on every run.
func Setup(ctx context.Context, env Env, o Options) error {
	agents, named, err := selectAgents(env, o)
	if err != nil {
		return err
	}
	var notes []string
	failed := false
	for _, a := range agents {
		if !a.Installs() {
			failed = failed || named
			continue
		}
		ok, stepFailed, note := install(ctx, env, a, o)
		notes = append(notes, note)
		failed = failed || stepFailed || named && !ok
	}
	blockNote := ""
	if o.RepoDir != "" {
		wroteBlock, wroteImport, err := WriteBlock(o.RepoDir)
		switch {
		case err != nil:
			notes = append(notes, "AGENTS.md: failed: "+err.Error())
			failed = true
		case wroteBlock && wroteImport:
			blockNote = " (setup just wrote it, and @AGENTS.md in CLAUDE.md)"
		case wroteBlock:
			blockNote = " (setup just wrote it)"
		case wroteImport:
			blockNote = " (setup just wrote @AGENTS.md in CLAUDE.md)"
		}
	}
	// What setup installed, for the notice to know without asking the agents.
	RecordPlugins(ctx, env)

	fmt.Fprintln(o.Out)
	if report(ctx, env, o, agents, blockNote) {
		notes = append(notes, "To update a copy of the skills, see "+readmeURL+"#updating-and-removing.")
	}
	if len(notes) > 0 {
		fmt.Fprintln(o.Out)
		for _, n := range notes {
			fmt.Fprintln(o.Out, n)
		}
	}
	if failed {
		return errors.New("the anfra skills are not set up for every agent: see above")
	}
	return nil
}

// install runs an agent's install steps, and reports whether the plugin ends
// up installed, whether a step failed, and what to tell the user.
func install(ctx context.Context, env Env, a Agent, o Options) (ok, failed bool, note string) {
	if _, err := env.LookPath(a.plugin.bin); err != nil {
		return false, false, fmt.Sprintf("%s: not installed, as its CLI `%s` is not on your PATH. Install %s, then run this again.", a.Name, a.plugin.bin, a.Name)
	}
	fmt.Fprintf(o.Out, "Installing %s for %s...\n", Plugin, a.Name)
	for _, args := range a.plugin.install {
		if err := env.Run(ctx, a.plugin.bin, args...); err != nil {
			return false, true, fmt.Sprintf("%s: failed: `%s %s`: %v", a.Name, a.plugin.bin, strings.Join(args, " "), err)
		}
	}
	return true, false, a.plugin.note
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

// Check reports what setup would act on, without changing anything, as
// report does.
func Check(ctx context.Context, env Env, o Options) error {
	agents, _, err := selectAgents(env, o)
	if err != nil {
		return err
	}
	report(ctx, env, o, agents, "")
	fmt.Fprintln(o.Out, "\nRun `anfra agent setup` to install or update them.")
	return nil
}

// report prints the state of the agents' skills:
//   - Agents: where each agent's skills come from: the plugin, through its
//     CLI, or copies in the folders it reads; or nowhere, with where to
//     install them from.
//   - Skills: each plugin and copy found, against the latest skills version
//     from the background check.
//   - Deprecated skills: copies of the names from before the prefix, to clean
//     up, as an agent loads them next to the new ones.
//   - AGENTS.md, in an anfra repo: its anfra block, with blockNote after its
//     state.
//
// It says whether a copy, rather than a plugin, is behind: setup doesn't
// update those.
func report(ctx context.Context, env Env, o Options, agents []Agent, blockNote string) (copiesBehind bool) {
	latest, checkedAt, known := update.CachedSkills()
	switch {
	case !known:
		fmt.Fprintln(o.Out, "Latest anfra skills: not known yet. anfra checks GitHub in the background, twice a day.")
	case sameDay(checkedAt, time.Now()):
		fmt.Fprintf(o.Out, "Latest anfra skills: %s, from GitHub (checked today)\n", latest)
	default:
		fmt.Fprintf(o.Out, "Latest anfra skills: %s, from GitHub (checked %s)\n", latest, checkedAt.Format("2 Jan"))
	}

	copies := Copies(env, o.RepoDir)
	type row struct{ where, status, details string }
	var skills []row
	var agentRows [][3]string
	for _, a := range agents {
		var how, details []string
		plugins := a.InstalledPlugin(ctx, env)
		if len(plugins) > 0 {
			how = append(how, "plugin")
		}
		for _, p := range plugins {
			where := a.Name + " plugin"
			if p.Scope != "user" {
				where += " (" + p.Scope + " scope)"
			}
			st, d := status(map[string][]string{p.Version: {Plugin}}, latest, known)
			skills = append(skills, row{where, st, d})
			details = append(details, strings.TrimSpace("plugin "+p.Version))
		}
		var dirs []string
		for _, c := range a.readsCopies(env, o.RepoDir, copies) {
			dirs = append(dirs, c.Dir)
		}
		if len(dirs) > 0 {
			how = append(how, "skills")
			details = append(details, strings.Join(dirs, ", "))
		}
		if len(how) == 0 {
			agentRows = append(agentRows, [3]string{a.Name, "none", "see " + a.README()})
			continue
		}
		agentRows = append(agentRows, [3]string{a.Name, strings.Join(how, ", "), strings.Join(details, "; ")})
	}

	fmt.Fprintln(o.Out, "\nAgents:")
	printRows(o.Out, agentRows)

	var deprecated []string
	for _, c := range copies {
		if len(c.Versions) > 0 {
			st, d := status(c.Versions, latest, known)
			skills = append(skills, row{c.Dir, st, d})
			copiesBehind = copiesBehind || st == "behind" || st == "mixed"
		}
		if c.OldNames {
			deprecated = append(deprecated, c.Dir)
		}
	}
	fmt.Fprintln(o.Out, "\nSkills:")
	if len(skills) == 0 {
		fmt.Fprintln(o.Out, "  none found")
	}
	var skillRows [][3]string
	for _, r := range skills {
		skillRows = append(skillRows, [3]string{r.where, r.status, r.details})
	}
	printRows(o.Out, skillRows)

	if len(deprecated) > 0 {
		fmt.Fprintf(o.Out, "\nDeprecated skills, from before %s:\n", renamedIn)
		for _, d := range deprecated {
			fmt.Fprintf(o.Out, "  %s\n", d)
		}
		fmt.Fprintf(o.Out, "Clean them up as %s#updating-and-removing says.\n", readmeURL)
	}

	if o.RepoDir != "" {
		b, err := CheckBlock(o.RepoDir)
		fmt.Fprintln(o.Out, "\nAGENTS.md:")
		switch {
		case err != nil:
			fmt.Fprintf(o.Out, "  can't read it: %v\n", err)
		case b.State == BlockMissing:
			fmt.Fprintln(o.Out, "  the anfra block is missing")
		case b.State == BlockOutdated:
			fmt.Fprintln(o.Out, "  the anfra block is out of date")
		default:
			fmt.Fprintln(o.Out, "  the anfra block is up-to-date"+blockNote)
		}
		if b.NoImport {
			fmt.Fprintln(o.Out, "  CLAUDE.md doesn't import it (@AGENTS.md)")
		}
	}

	fmt.Fprintln(o.Out, "\nanfra only looks in the folders above, so a copy somewhere else won't be shown.")
	return copiesBehind
}

// printRows prints rows of three columns, aligned.
func printRows(w io.Writer, rows [][3]string) {
	var w0, w1 int
	for _, r := range rows {
		w0, w1 = max(w0, len(r[0])), max(w1, len(r[1]))
	}
	for _, r := range rows {
		fmt.Fprintf(w, "  %-*s  %-*s  %s\n", w0, r[0], w1, r[1], r[2])
	}
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Local().Date()
	by, bm, bd := b.Local().Date()
	return ay == by && am == bm && ad == bd
}
