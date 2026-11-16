package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/logging"
	"github.com/holistics/anfra/internal/pretty"
	"github.com/holistics/anfra/internal/repo"
)

// logsResult is logs' result: the entries of the repo's log asked for.
type logsResult struct {
	Message string          `json:"message" doc:"the outcome, in a sentence: which entries of which log file"`
	Entries []logging.Entry `json:"entries" pretty:",view=log_entries" doc:"the entries, oldest first"`
	Hints   []string        `json:"hints,omitempty" doc:"what to do next, if any"`
}

// logsFlags are logs' flags.
type logsFlags struct {
	lines  int
	follow bool
	level  string
	events []string
	since  string
}

// newLogsCmd shows the repo's log (docs/usage/configs.md#logs): anfra's own
// records, its sidecars', and its Data Apps' reports, in one file per repo.
func newLogsCmd() *cobra.Command {
	var fl logsFlags
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show the repo's log: what anfra, its sidecars and its Data Apps recorded",
		Long: "Show the repo's log: what anfra, its sidecars and its Data Apps recorded, the last 50 entries\n" +
			"by default, oldest first. Every command in the repo, and anfra serve, writes to it.\n\n" +
			"--level, --event and --since narrow the entries, and -n counts those left. A sidecar's own line\n" +
			"has no level or event, so either of them leaves it out. --event names anfra's events, such as\n" +
			"query_cache.unavailable; a name before a dot takes every event under it (query_cache).\n\n" +
			"-f then shows each entry as it's written, until Ctrl-C. It takes no --yaml or --json, which\n" +
			"write one result, and -f never ends.",
		Example: "  anfra logs --level warn\n" +
			"  anfra logs --event query_cache --since 1h\n" +
			"  anfra logs -f",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			filter, err := fl.filter(time.Now())
			if err != nil {
				return err
			}
			dir, err := os.Getwd()
			if err != nil {
				return err
			}
			r := repo.Resolve(dir)
			path := filepath.Join(r.LogsDir(), logging.FileName)
			entries, offset, err := logging.Read(path, filter, fl.lines)
			if err != nil {
				return fmt.Errorf("read the repo's log: %w", err)
			}
			res := fl.result(path, entries)
			if len(entries) == 0 && fl.asksForApps() { // in place of dropping the filters, which shows other entries
				res.Hints = appReportHints(cmd.Context(), r)
			}
			if err := writeResult(cmd, res); err != nil {
				return err
			}
			if !fl.follow {
				return nil
			}
			out := cmd.OutOrStdout()
			styles := cliui.NewTerminalStyles(out)
			// Indented as the entries above it, under their label.
			return logging.Follow(cmd.Context(), path, offset, filter, func(e logging.Entry) error {
				return pretty.RenderView(out, "log_entries", []logging.Entry{e}, 2, styles)
			})
		},
	}
	f := cmd.Flags()
	f.IntVarP(&fl.lines, "lines", "n", 50, "how many entries to show, the last ones: 0 for every one")
	f.BoolVarP(&fl.follow, "follow", "f", false, "then show each entry as it's written, until Ctrl-C")
	f.StringVar(&fl.level, "level", "", "only entries at this level or above: debug, info, warn or error")
	f.StringArrayVar(&fl.events, "event", nil, "only this event, or the events under it, such as query_cache; repeat it for several")
	f.StringVar(&fl.since, "since", "", "only entries written since then: a duration back from now, such as 10m or 2h, or a time, such as 2026-11-16T09:00:00Z")
	resultIs[logsResult](cmd)
	markPrettyOutput(cmd)
	return cmd
}

// filter is the entries the flags ask for, or the flag they refuse.
func (fl logsFlags) filter(now time.Time) (logging.Filter, error) {
	f := logging.Filter{Level: fl.level, Events: fl.events}
	switch {
	case fl.lines < 0:
		return f, refused("lines", "invalid", fmt.Sprintf("-n is how many entries to show, 0 for every one, not %d.", fl.lines))
	case fl.follow && structuredAsked():
		flag := structuredFlag()
		return f, refused("follow", "invalid", fmt.Sprintf("anfra logs doesn't take -f with %s, because %s writes one result, and -f never ends. Drop -f, or drop %s to follow the log as it's written.", flag, flag, flag))
	case fl.level != "" && !logging.LevelValid(fl.level):
		return f, refused("level", "invalid", fmt.Sprintf("--level is debug, info, warn or error, not %q.", fl.level))
	}
	if fl.since != "" {
		if d, err := time.ParseDuration(fl.since); err == nil && d >= 0 {
			f.Since = now.Add(-d)
		} else if t, err := time.Parse(time.RFC3339Nano, fl.since); err == nil {
			f.Since = t
		} else {
			return f, refused("since", "invalid", fmt.Sprintf("--since is a duration back from now, such as 10m or 2h, or a time, such as 2026-11-16T09:00:00Z, not %q.", fl.since))
		}
	}
	return f, nil
}

// result says which entries of the log at path these are.
func (fl logsFlags) result(path string, entries []logging.Entry) logsResult {
	where := homeRelative(path)
	narrowed := fl.level != "" || len(fl.events) > 0 || fl.since != ""
	res := logsResult{Entries: entries}
	if res.Entries == nil {
		res.Entries = []logging.Entry{}
	}
	switch {
	case len(entries) == 0 && !narrowed:
		res.Message = fmt.Sprintf("Nothing is logged yet in %s.", where)
	case len(entries) == 0:
		res.Message = fmt.Sprintf("No entry in %s matches.", where)
		res.Hints = []string{"Drop --level, --event or --since to see every entry."}
	case fl.lines == 1 && len(entries) == 1:
		res.Message = fmt.Sprintf("The last entry of %s%s.", where, matching(narrowed, 1))
	case fl.lines > 0 && len(entries) == fl.lines:
		res.Message = fmt.Sprintf("The last %d entries of %s%s.", len(entries), where, matching(narrowed, len(entries)))
	case narrowed:
		res.Message = fmt.Sprintf("The %d %s of %s that %s.", len(entries), plural(len(entries), "entry", "entries"), where, plural(len(entries), "matches", "match"))
	default:
		res.Message = fmt.Sprintf("All %d %s of %s.", len(entries), plural(len(entries), "entry", "entries"), where)
	}
	if fl.follow {
		res.Message = strings.TrimSuffix(res.Message, ".") + ", then each as it's written. Ctrl-C stops."
	}
	return res
}

// asksForApps reports whether the flags ask for the Data Apps' reports: an
// event under app.
func (fl logsFlags) asksForApps() bool {
	for _, e := range fl.events {
		if e == "app" || strings.HasPrefix(e, "app.") {
			return true
		}
	}
	return false
}

// appReportHints say why the repo's log holds no Data App report: anfra serve,
// which logs them, runs with ANFRA_NO_APP_LOGS, or no app was opened in it
// yet, or it doesn't run in this repo but in another, or nowhere.
func appReportHints(ctx context.Context, r repo.Local) []string {
	if srv, ok := findServer(ctx, r); ok {
		if srv.Config.NoAppLogs {
			return []string{"anfra serve runs in this repo with ANFRA_NO_APP_LOGS=1, so it doesn't log its Data Apps' reports. Read the inspect panel instead, or ask the user to."}
		}
		return []string{"anfra serve runs in this repo, and logs a Data App's reports once the app is open. Open it, use it, then run this again."}
	}
	var elsewhere []string
	for _, s := range runningServers(ctx) {
		if s.RepoID != r.ID {
			elsewhere = append(elsewhere, homeRelative(s.RepoDir))
		}
	}
	if len(elsewhere) > 0 {
		return []string{fmt.Sprintf("anfra serve doesn't run in this repo, so its log has no Data App's reports. It runs in %s: run `anfra logs` there.", strings.Join(elsewhere, " and "))}
	}
	return []string{"anfra serve isn't running, and only it logs a Data App's reports. Run `anfra serve` in this repo, open the app, then run this again."}
}

// matching says the entries are those the filters kept, when they narrowed
// them.
func matching(narrowed bool, n int) string {
	if narrowed {
		return " that " + plural(n, "matches", "match")
	}
	return ""
}
