// Package status is the commands that report on anfra itself: version, and
// status of the warm server and its sidecars.
package status

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"golang.org/x/mod/semver"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/logging"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/internal/repo"
)

// Version prints the anfra version and, given the version of the anfra skills
// in use, compares both with the latest the host knows of, saying what to act
// on as hints (docs/designs/agent-setup.md). It reads only what the host
// cached: no network, no files.
var Version = command.Define(command.Def[VersionInput, VersionResult]{
	Name:  "version",
	Short: "Print the anfra version, and say when anfra or the anfra skills are behind",
	Long: "Print the anfra version, and say when anfra or the anfra skills are behind.\n\n" +
		"For this repo's server, and whether it is running, see `anfra status`.",
	ReadOnly: true,
	Check:    checkVersion,
	// No Needs: pure metadata, spawns nothing.
	Run: func(_ context.Context, cc command.CommandContext, in VersionInput) (VersionResult, error) {
		return runVersion(cc, in, time.Now()), nil
	},
})

// SkillsUnknown is the skills arg when the version of the skills in use isn't
// known: a skill with no stamp, as copies from before the stamp have none.
const SkillsUnknown = "unknown"

// staleAfter is how old the host's check gets before the answer gives its date.
const staleAfter = 48 * time.Hour

func checkVersion(in VersionInput) error {
	if in.Skills != "" && in.Skills != SkillsUnknown && !validVersion(in.Skills) {
		return command.InvalidArg("skills", "invalid", fmt.Sprintf("%q is not a version such as 0.16.0, nor %s", in.Skills, SkillsUnknown))
	}
	return nil
}

func runVersion(cc command.CommandContext, in VersionInput, now time.Time) VersionResult {
	r := VersionResult{Version: meta.Version}
	if cc.Update != nil {
		r.Update = cc.Update()
	}
	if r.Update != nil && r.Update.Available {
		r.Hints = append(r.Hints, fmt.Sprintf("anfra %s is out: ask the user to run `anfra update`.", r.Update.Latest))
	}
	if in.Skills != "" {
		var known *command.SkillsInfo
		if cc.Skills != nil {
			known = cc.Skills()
		}
		r.Skills, r.Hints = compareSkills(in.Skills, known, now, r.Hints)
	}
	return r
}

// compareSkills compares the skills version in use with the latest known, and
// adds what to act on to hints. Latest is never older than the version in use:
// a newer one only means the check is old. So a program tells "behind" by the
// two differing, with no version comparison of its own.
func compareSkills(inUse string, known *command.SkillsInfo, now time.Time, hints []string) (*SkillsResult, []string) {
	s := &SkillsResult{}
	if inUse != SkillsUnknown {
		s.Version = inUse
	}
	if known == nil || known.Latest == "" {
		return s, append(hints, "The latest anfra skills version isn't known yet: anfra checks in the background, twice a day.")
	}
	s.Latest = known.Latest
	if now.Sub(known.CheckedAt) > staleAfter {
		s.Checked = known.CheckedAt.Format(time.DateOnly)
	}
	switch {
	case inUse == SkillsUnknown:
		hints = append(hints, "To see which copies of the anfra skills are installed, and their versions, run `anfra agent setup check`.")
	case newer(inUse, s.Latest):
		s.Latest = inUse
	case inUse != s.Latest:
		hints = append(hints, fmt.Sprintf("anfra skills %s are behind the latest, %s: run `anfra agent setup --agent <your id>`.", inUse, s.Latest))
	}
	return s, hints
}

// validVersion reports whether v is a full version, 1.2.3, without a prefix:
// semver also takes 1.2, which Canonical fills in, and so tells apart.
func validVersion(v string) bool { return semver.Canonical("v"+v) == "v"+v }

func newer(a, b string) bool {
	return validVersion(a) && validVersion(b) && semver.Compare("v"+a, "v"+b) > 0
}

// Status reports the repo, and whether a warm server is running and its
// sidecars are healthy.
var Status = command.Define(command.Def[command.NoInput, StatusResult]{
	Name:  "status",
	Short: "Report the repo, and whether a warm server is running and its sidecars are healthy",
	Long: "Report the repo, and whether a warm server is running and its sidecars are healthy.\n\n" +
		"For this anfra's own version, and whether it is behind, see `anfra version`.",
	ReadOnly: true,
	// No Needs on purpose: status must NOT spawn sidecars. One-shot (no warm
	// server) then honestly reports not_running instead of starting the
	// sidecars just to declare them healthy.
	Run: func(ctx context.Context, cc command.CommandContext, _ command.NoInput) (StatusResult, error) {
		r := StatusResult{Repo: repoStatus(cc.Repo), Server: checkStatus(ctx, cc.Clients)}
		if s := cc.Server; s != nil {
			r.Server.URL, r.Server.InstanceID, r.Server.Version = s.URL, s.InstanceID, s.Version
		}
		return r, nil
	},
	Valid: func(r StatusResult) bool { return r.Server.State == StateHealthy },
})

// VersionInput is the version of the anfra skills in use, to compare.
type VersionInput struct {
	Skills string `json:"skills,omitempty" doc:"the version of the anfra skills in use, from the line ending each skill (anfra skills <version>), or unknown when a skill has no such line; adds the latest skills version to the answer"`
}

func (VersionInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[VersionInput](s)
}

// VersionResult is the `version` result. The CLI prints its keys in
// alphabetical order, so skills' latest: comes before update's: the skills'
// session hook, which reads the first latest: line as anfra's, gets skills only
// when it passes --skills.
type VersionResult struct {
	Version string `json:"version"`
	// Update is the newest release the host knows of; absent when it does not
	// check for updates or has not checked yet.
	Update *command.UpdateInfo `json:"update,omitempty"`
	// Skills compares the skills version given with the latest; absent when
	// none was given.
	Skills *SkillsResult `json:"skills,omitempty"`
	// Hints are what to act on, for an agent to follow; absent when nothing is.
	Hints []string `json:"hints,omitempty"`
}

// SkillsResult is the anfra skills in use against the latest known.
type SkillsResult struct {
	// Version is the skills version given; absent for unknown.
	Version string `json:"version,omitempty"`
	// Latest is the newest skills version known, never older than Version: they
	// differ only when Version is behind. Absent when none is known yet.
	Latest string `json:"latest,omitempty"`
	// Checked is the date of the host's last check, when it is more than two
	// days old.
	Checked string `json:"checked,omitempty"`
}

// StatusResult is the `status` result: the repo, and its warm server.
type StatusResult struct {
	Repo   RepoStatus   `json:"repo"`
	Server ServerStatus `json:"server"`
}

// RepoStatus is the repo the command runs on.
type RepoStatus struct {
	ID string `json:"id" doc:"the repo's id, which names its state folder: <ANFRA_HOME>/repos/<id>"`
	// Logs is the repo's log file; absent for a repo with no directory (a
	// managed repo), whose host keeps its logs.
	Logs string `json:"logs,omitempty" doc:"the repo's log file: anfra's and its sidecars' log; absent for a managed repo"`
}

// ServerStatus is the warm server's state and, when it is running, where it is,
// its version and its sidecars' health.
type ServerStatus struct {
	State      State          `json:"state" enum:"healthy,degraded,not_running"`
	URL        string         `json:"url,omitempty"`
	InstanceID string         `json:"instance_id,omitempty"`
	Version    string         `json:"version,omitempty"`
	Sidecars   *SidecarHealth `json:"sidecars,omitempty"`
}

// State is the warm server's state.
type State string

const (
	// StateHealthy: the server is running, and every sidecar answers.
	StateHealthy State = "healthy"
	// StateDegraded: the server is running, and a sidecar does not answer.
	StateDegraded State = "degraded"
	// StateNotRunning: no warm server.
	StateNotRunning State = "not_running"
)

// SidecarHealth is each sidecar's health: "ok" or the error message.
type SidecarHealth struct {
	Node       string `json:"node"`
	CanalQuery string `json:"canal-query"`
}

// checkStatus reports the warm server's health. status spawns no sidecars, so in
// one-shot mode (no warm server) both clients are nil → not_running; under a
// warm server the clients are live and get health-checked.
func checkStatus(ctx context.Context, c command.Clients) ServerStatus {
	if c.Node == nil && c.CanalQuery == nil {
		return ServerStatus{State: StateNotRunning}
	}
	sc := &SidecarHealth{Node: "ok", CanalQuery: "ok"}
	if c.Node == nil {
		sc.Node = "unavailable"
	} else if _, err := c.Node.Ping(ctx); err != nil {
		sc.Node = err.Error()
	}
	if c.CanalQuery == nil {
		sc.CanalQuery = "unavailable"
	} else if err := c.CanalQuery.Health(ctx); err != nil {
		sc.CanalQuery = err.Error()
	}
	state := StateHealthy
	if sc.Node != "ok" || sc.CanalQuery != "ok" {
		state = StateDegraded
	}
	return ServerStatus{State: state, Sidecars: sc}
}

// repoStatus is r's id and, for a local repo, its log file.
func repoStatus(r repo.Repo) RepoStatus {
	s := RepoStatus{ID: r.ID()}
	if local, ok := r.Local(); ok {
		s.Logs = filepath.Join(local.LogsDir(), logging.FileName)
	}
	return s
}
