package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/internal/update"
	"github.com/spf13/cobra"
)

// newUpdateCmd is the manual self-update command (like `gh`/`deno upgrade`):
// check the latest GitHub release and, unless --check, replace this binary.
func newUpdateCmd() *cobra.Command {
	var checkOnly bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update anfra to the latest release (use --check to only report)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpdate(cmd.Context(), checkOnly)
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "only check for an update; do not install")
	return cmd
}

func runUpdate(ctx context.Context, checkOnly bool) error {
	// ctx is the signal-cancelable root from main, so Ctrl-C aborts the download
	// (its request is context-aware). Per-request timeouts are bounded inside the
	// update package (a quick lookup, then a large download).
	rel, err := update.Latest(ctx)
	if err != nil {
		return err
	}
	// Record the check so the background notice stays quiet right after.
	update.RecordCheck(rel)

	if !rel.IsNewer() {
		fmt.Printf("anfra is up to date (%s).\n", meta.Version)
		return nil
	}
	if checkOnly {
		fmt.Printf("Update available: anfra %s (you have %s). Run `anfra update` to install.\n", rel.Version, meta.Version)
		return nil
	}

	fmt.Printf("Downloading anfra %s...\n", rel.Version)
	// Show a progress line only on an interactive terminal; stay silent when
	// output is piped/captured (agent, CI).
	var progress io.Writer
	if stderrIsInteractive() {
		progress = os.Stderr
	}
	if err := update.Apply(ctx, rel, progress); err != nil {
		return err
	}
	fmt.Printf("Updated anfra %s -> %s.\n", meta.Version, rel.Version)
	return nil
}

// newUpdateCheckCmd is a hidden command run detached in the background to
// refresh the cached update check without blocking the foreground command.
func newUpdateCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "__update-check",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return update.Refresh(cmd.Context())
		},
	}
}

// commands for which the after-the-command notice is suppressed: they do their
// own checking (serve announces updates while it runs, see watchUpdates) or are
// internal.
var noNotifyCommands = map[string]bool{"update": true, "__update-check": true, "serve": true}

// updateNotifyDisabled reports whether the background update notice is opted out.
func updateNotifyDisabled() bool {
	return os.Getenv("ANFRA_NO_UPDATE_NOTIFIER") != ""
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

// watchUpdates announces a newer release while serve runs: at startup and then
// daily, so a server left up for days still hears of one. It runs off the
// request path and announces each version once. Never auto-updates: the running
// server would keep the old version anyway. Silent on any error (offline, rate
// limited).
func watchUpdates(ctx context.Context, logger *slog.Logger) {
	announced := ""
	for {
		if r, err := update.Check(ctx); err == nil && r.IsNewer() && r.Version != announced {
			announced = r.Version
			logger.Info("serve.update_available", "latest", r.Version, "current", meta.Version)
			fmt.Fprintln(os.Stderr, update.Notice(r))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(update.CheckEvery):
		}
	}
}

// autoUpdateEnabled reports whether opt-in fully-automatic update is on. When set,
// a known-newer version is applied in a detached background process (effective on
// the next run) instead of only printing a notice.
func autoUpdateEnabled() bool {
	v := os.Getenv("ANFRA_AUTO_UPDATE")
	return v != "" && v != "0" && v != "false"
}

// maybeNotifyUpdate prints a cached "update available" notice (to stderr, so it
// never pollutes command output). If opt-in auto-update is on, it instead applies
// the update in a detached background process. When the cache is stale it spawns a
// detached refresh so the next run's notice is current. Best-effort and silent on
// any error — an update check must never break a command.
func maybeNotifyUpdate(invoked string) {
	if updateNotifyDisabled() || noNotifyCommands[invoked] {
		return
	}
	notice := update.CachedNotice()

	// Opt-in auto-update is explicit, so it runs regardless of interactivity
	// (e.g. a service that set ANFRA_AUTO_UPDATE=1).
	if notice != "" && autoUpdateEnabled() {
		fmt.Fprintln(os.Stderr, "\n"+notice+" (auto-updating in the background)")
		spawnDetached("update")
		return
	}

	// The passive notice and its background refresh are for interactive humans
	// only. When output is piped/captured — an agent calling anfra, CI, a script —
	// stay completely silent and spawn nothing, so we add no noise or overhead.
	if !stderrIsInteractive() {
		return
	}
	if notice != "" {
		fmt.Fprintln(os.Stderr, "\n"+notice)
	}
	if update.Stale() {
		spawnDetached("__update-check")
	}
}

// stderrIsInteractive reports whether stderr is a terminal (not a pipe/file).
func stderrIsInteractive() bool {
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
