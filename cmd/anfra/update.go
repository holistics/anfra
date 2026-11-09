package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/envflag"

	"github.com/holistics/anfra/internal/agents"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/internal/update"
	"github.com/spf13/cobra"

	"github.com/holistics/anfra/shared/logkit"
)

// newUpdateCmd is the manual self-update command (like `gh`/`deno upgrade`):
// check the latest GitHub release and, unless --check, replace this binary.
// It warns when the agents' skills need attention, which `anfra agent setup`
// handles (docs/designs/agent-setup.md).
func newUpdateCmd() *cobra.Command {
	var checkOnly bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update anfra to the latest release",
		Long: "Update anfra to the latest release, replacing this binary. `anfra update check` only checks.\n\n" +
			"Its result is `anfra version`'s answer, checked after the update.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if checkOnly {
				printWarning(cmd.ErrOrStderr(), "deprecated", "--check is deprecated. Use `anfra update check` instead.")
			}
			return updateRun(cmd, checkOnly)
		},
	}
	// --check was update check's name: it still works, warning.
	cmd.Flags().BoolVar(&checkOnly, "check", false, "former name of `anfra update check`")
	_ = cmd.Flags().MarkHidden("check")
	markPrettyOutput(cmd)
	resultIs[updateResult](cmd)
	check := &cobra.Command{
		Use:   "check",
		Short: "Check for a newer release, without installing it",
		Long: "Check for a newer release, asking GitHub now rather than reading the background check, and\n" +
			"install nothing. It also checks the latest anfra skills version, for\n" +
			"`anfra version --skills` to compare with.\n\n" +
			"Its result is `anfra version`'s answer, checked now.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return updateRun(cmd, true) },
	}
	markPrettyOutput(check)
	resultIs[updateResult](check)
	cmd.AddCommand(check)
	return cmd
}

// updateRun runs update, or update check, and writes its result; the skills
// needing attention is a warning.
func updateRun(cmd *cobra.Command, checkOnly bool) error {
	res, err := runUpdate(cmd.Context(), checkOnly, cmd.ErrOrStderr())
	if err != nil {
		return err
	}
	if line := agents.Attention(agents.System(), repoDirIfAny()); line != "" {
		printWarning(cmd.ErrOrStderr(), "skills_need_attention", line)
	}
	return writeResult(cmd, res)
}

// updateResult is update's result: version's answer as the update leaves it,
// and what it replaced (docs/designs/commands-and-api.md#new-state-as-output).
type updateResult struct {
	Message   string             `json:"message" doc:"the outcome, in a sentence"`
	Version   string             `json:"version" doc:"the anfra version installed after the command"`
	Update    command.UpdateInfo `json:"update" doc:"the latest release, checked now"`
	Previous  string             `json:"previous,omitempty" doc:"the version the update replaced"`
	Changelog string             `json:"changelog" doc:"the changelog as of the latest release: what it brings, and every release before it"`
	Hints     []string           `json:"hints,omitempty" doc:"what to do next, if any"`
}

// runUpdate checks for the latest release and, unless checkOnly, installs it.
// What it says for a person, its progress and the release notes, goes to
// errOut on a terminal only; a running server left on another version is a
// warning.
func runUpdate(ctx context.Context, checkOnly bool, errOut io.Writer) (updateResult, error) {
	if meta.FromSource() {
		return updateResult{}, clierr.Precondition("`anfra update` doesn't update an anfra built from source (%s), because it would replace your build with a release. "+
			"Rebuild it instead, or install a release.", meta.Version)
	}
	// ctx is the signal-cancelable root from main, so Ctrl-C aborts the download
	// (its request is context-aware). Per-request timeouts are bounded inside the
	// update package (a quick lookup, then a large download).
	rel, err := update.Latest(ctx)
	if err != nil {
		return updateResult{}, err
	}
	// Record the check so the background notice stays quiet right after, and
	// check the skills too, so the whole cache is current: best-effort, as the
	// result doesn't report them.
	update.RecordCheck(rel)
	_ = update.RefreshSkills(ctx)

	res := updateResult{
		Version:   meta.Version,
		Update:    command.UpdateInfo{Latest: rel.Version, Available: rel.IsNewer()},
		Changelog: update.ChangelogURL(rel.Version),
	}
	terminal := stderrIsInteractive()
	if !rel.IsNewer() {
		res.Message = fmt.Sprintf("anfra %s is up to date.", meta.Version)
		return res, nil
	}
	if checkOnly {
		if terminal {
			showReleaseNotes(ctx, errOut, meta.Version, rel.Version)
		}
		res.Message = fmt.Sprintf("anfra %s is available (you have %s).", rel.Version, meta.Version)
		res.Hints = []string{"Run `anfra update` to install it."}
		return res, nil
	}

	var progress io.Writer
	if terminal {
		progress = errOut
		fmt.Fprintf(errOut, "Downloading anfra %s...\n", rel.Version)
	}
	if err := update.Apply(ctx, rel, progress); err != nil {
		return updateResult{}, err
	}
	if terminal {
		showReleaseNotes(ctx, errOut, meta.Version, rel.Version)
	}
	// A running server keeps the binary it started with: last, so the notes
	// don't bury it.
	for _, srv := range runningServers(ctx) {
		if srv.Version != rel.Version {
			printWarning(errOut, "server_version_mismatch", fmt.Sprintf(
				"`anfra serve` %s is still running for %s. Restart it to run %s.", srv.Version, srv.RepoDir, rel.Version))
		}
	}
	res.Message = fmt.Sprintf("Updated anfra %s → %s.", meta.Version, rel.Version)
	res.Version, res.Previous = rel.Version, meta.Version
	res.Update.Available = false
	return res, nil
}

// maxNotes caps how many releases' notes showReleaseNotes prints; the earlier
// ones are a link away.
const maxNotes = 3

// showReleaseNotes prints to w what the releases after from, up to and
// including to, changed, newest first. Best-effort: when the notes can't be
// fetched, it points to them instead, and never fails the command.
func showReleaseNotes(ctx context.Context, w io.Writer, from, to string) {
	notes, err := update.ReleaseNotes(ctx, from, to)
	if err != nil || len(notes) == 0 {
		fmt.Fprintf(w, "See what changed: %s\n", update.ReleasesURL)
		return
	}
	update.RenderNotes(w, notes, maxNotes, cliui.NewTerminalStyles(w), cliui.Width(w))
}

// newUpdateCheckCmd is a hidden command run detached in the background to
// refresh the cached update check without blocking the foreground command.
func newUpdateCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "__update-check",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := update.Refresh(cmd.Context())
			// The plugins installed, for the notice to read without asking the
			// agents' CLIs, which start slowly.
			agents.RecordPlugins(cmd.Context(), agents.System())
			return err
		},
	}
}

// commands for which the after-the-command notice is suppressed: they do their
// own checking (serve announces updates while it runs, see checkUpdates) or are
// internal.
var noNotifyCommands = map[string]bool{"update": true, "__update-check": true, "serve": true}

// updateNotifyDisabled reports whether the background update notice is opted out.
func updateNotifyDisabled() bool {
	return envflag.On(envflag.NoUpdateNotifier)
}

// knownUpdate is the newest release from the cached check, for the version
// command to report; nil when nothing has been checked yet. No network.
func knownUpdate() *command.UpdateInfo {
	r := update.Cached()
	if r == nil {
		return nil
	}
	return &command.UpdateInfo{Latest: r.Version, Available: r.IsNewer()}
}

// knownSkills is the latest anfra skills version from the cached check, for
// the version command to compare with; nil when nothing has been checked yet,
// or the check failed. No network.
func knownSkills() *command.SkillsInfo {
	latest, at, ok := update.CachedSkills()
	if !ok {
		return nil
	}
	return &command.SkillsInfo{Latest: latest, CheckedAt: at}
}

// checkUpdates announces a newer release while serve runs: at startup and then
// hourly, so a server left up for days still hears of one. It is a periodic
// routine's run: a fresh cache costs a read, a stale one a lookup. It runs off the request path and announces each version
// once. Never auto-updates: the running server would keep the old version
// anyway. Silent on any error (offline, rate limited).
func checkUpdates(logger *slog.Logger) func(context.Context) error {
	announced := ""
	return func(ctx context.Context) error {
		if r, err := update.Check(ctx); err == nil && r.IsNewer() && r.Version != announced {
			announced = r.Version
			logger.Info("Update available", logkit.EventKey, eventServeUpdateAvailable, "latest", r.Version, "current", meta.Version, "notice", update.Notice(r))
		}
		return nil
	}
}

// autoUpdateEnabled reports whether opt-in fully-automatic update is on. When set,
// a known-newer version is applied in a detached background process (effective on
// the next run) instead of only printing a notice.
func autoUpdateEnabled() bool {
	return envflag.On(envflag.AutoUpdate)
}

// maybeNotifyUpdate is how notices are shown outside a console: after the
// command, on stderr, each in full, so they follow what the person asked for
// and never mix into a result. It's also where what they're worked out from is
// kept current: a detached refresh when it's gone stale, for the next run to
// read. Best-effort and silent: a notice never breaks a command.
func maybeNotifyUpdate(invoked string) {
	if updateNotifyDisabled() || noNotifyCommands[invoked] {
		return
	}
	// The background refresh runs whoever reads (at most hourly), so what an
	// agent reads through `anfra version` and the skills' session hook stays
	// current. CI spawns nothing.
	defer func() {
		if update.Stale() && os.Getenv("CI") == "" {
			spawnDetached("__update-check")
		}
	}()
	// A console shows its notices under its prompt (consoleBadge), not after
	// each command's output. Notices are for a person at a terminal: piped or
	// captured (an agent calling anfra, a script), none are worked out, unless
	// opt-in auto-update needs anfra's.
	if output.inConsole || !stderrIsInteractive() && !autoUpdateEnabled() {
		return
	}
	for _, n := range notices() {
		switch {
		case n.update && autoUpdateEnabled():
			// Opt-in auto-update is explicit, so it runs regardless of
			// interactivity (a service that set ANFRA_AUTO_UPDATE=1); saying
			// so is for a person at a terminal, as the notices are.
			if stderrIsInteractive() {
				fmt.Fprintln(os.Stderr, "\n"+n.autoText)
			}
			spawnDetached("update")
			return
		case !stderrIsInteractive():
			return
		case n.skills:
			// At most once a day; not after agent setup (or its former name),
			// which has just reported it all.
			if invoked == "setup" || invoked == "install" || !agents.NoticeDue() {
				continue
			}
			agents.MarkNoticed()
		}
		fmt.Fprintln(os.Stderr, "\n"+n.text)
	}
}

// stderrIsInteractive reports whether stderr is a terminal (not a pipe/file),
// and the run may treat it as one: not with --non-interactive.
func stderrIsInteractive() bool {
	if output.nonInteractive {
		return false
	}
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// spawnDetached launches `anfra <args...>` detached from this process so it
// survives after the foreground command exits (the update-notifier pattern).
func spawnDetached(args ...string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(exe, args...) //nolint:gosec // fixed args, our own binary
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	detachProcess(cmd)
	if err := cmd.Start(); err == nil {
		_ = cmd.Process.Release()
	}
}
