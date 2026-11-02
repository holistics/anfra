package status

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
)

// status answers its verdict as state: not_running without a warm server,
// degraded when a sidecar does not answer, healthy when both do. Only healthy
// is valid.
func TestStatusState(t *testing.T) {
	canal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer canal.Close()
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer down.Close()

	for _, tc := range []struct {
		name    string
		clients command.Clients
		want    State
	}{
		{"no warm server", command.Clients{}, StateNotRunning},
		{"a sidecar not connected", command.Clients{CanalQuery: canalquery.NewClient(canal.URL, false)}, StateDegraded},
		{"a sidecar down", command.Clients{
			Node:       anfranode.NewClientHTTP(down.URL),
			CanalQuery: canalquery.NewClient(canal.URL, false),
		}, StateDegraded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := checkStatus(context.Background(), tc.clients)
			if got.State != tc.want {
				t.Errorf("state = %s, want %s (%+v)", got.State, tc.want, got.Sidecars)
			}
		})
	}
	if valid, _ := Status.Valid(StatusResult{State: StateDegraded}); valid {
		t.Error("degraded is valid")
	}
	if valid, _ := Status.Valid(StatusResult{State: StateHealthy}); !valid {
		t.Error("healthy is invalid")
	}
}

// compareSkills says what to act on, and never answers a latest older than the
// version in use, so a program tells "behind" by the two differing.
func TestCompareSkills(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	fresh := &command.SkillsInfo{Latest: "0.16.0", CheckedAt: now.Add(-time.Hour)}
	for _, tc := range []struct {
		name  string
		inUse string
		known *command.SkillsInfo
		want  SkillsResult
		hint  string // a hint's start; "" for none
	}{
		{"current", "0.16.0", fresh, SkillsResult{Version: "0.16.0", Latest: "0.16.0"}, ""},
		{"a minor behind", "0.15.0", fresh, SkillsResult{Version: "0.15.0", Latest: "0.16.0"}, "anfra skills 0.15.0 are behind the latest, 0.16.0"},
		{"a patch behind", "0.15.9", &command.SkillsInfo{Latest: "0.15.10", CheckedAt: now}, SkillsResult{Version: "0.15.9", Latest: "0.15.10"}, "anfra skills 0.15.9 are behind the latest, 0.15.10"},
		{"newer than the check", "0.16.1", fresh, SkillsResult{Version: "0.16.1", Latest: "0.16.1"}, ""},
		{"nothing checked", "0.16.0", nil, SkillsResult{Version: "0.16.0"}, "The latest anfra skills version isn't known yet"},
		{"an old check", "0.16.0", &command.SkillsInfo{Latest: "0.16.0", CheckedAt: now.Add(-72 * time.Hour)}, SkillsResult{Version: "0.16.0", Latest: "0.16.0", Checked: "2026-10-27"}, ""},
		{"version unknown", SkillsUnknown, fresh, SkillsResult{Latest: "0.16.0"}, "To see which copies of the anfra skills are installed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, hints := compareSkills(tc.inUse, tc.known, now, nil)
			if *got != tc.want {
				t.Errorf("skills = %+v, want %+v", *got, tc.want)
			}
			switch {
			case tc.hint == "" && len(hints) > 0:
				t.Errorf("hints = %q, want none", hints)
			case tc.hint != "" && (len(hints) != 1 || !strings.HasPrefix(hints[0], tc.hint)):
				t.Errorf("hints = %q, want one starting %q", hints, tc.hint)
			}
		})
	}
}

// Without --skills, version answers as before, plus the hint when anfra is
// behind; with it, the skills too.
func TestVersionHints(t *testing.T) {
	now := time.Now()
	cc := command.CommandContext{
		Update: func() *command.UpdateInfo { return &command.UpdateInfo{Latest: "9.0.0", Available: true} },
		Skills: func() *command.SkillsInfo { return &command.SkillsInfo{Latest: "0.16.0", CheckedAt: now} },
	}
	plain := runVersion(cc, VersionInput{}, now)
	if plain.Skills != nil || len(plain.Hints) != 1 || plain.Hints[0] != "anfra 9.0.0 is out: ask the user to run `anfra update`." {
		t.Errorf("without --skills: %+v", plain)
	}
	both := runVersion(cc, VersionInput{Skills: "0.15.0"}, now)
	if both.Skills == nil || both.Skills.Latest != "0.16.0" || len(both.Hints) != 2 {
		t.Errorf("with --skills: %+v", both)
	}

	// A host that doesn't check (an embedding platform, a user who opted out).
	if r := runVersion(command.CommandContext{}, VersionInput{Skills: "0.15.0"}, now); r.Update != nil || r.Skills.Latest != "" {
		t.Errorf("no checks: %+v", r)
	}
}

func TestVersionRefusesWhatIsntAVersion(t *testing.T) {
	for _, ok := range []string{"0.16.0", "1.0.0-rc.1", SkillsUnknown, ""} {
		if err := checkVersion(VersionInput{Skills: ok}); err != nil {
			t.Errorf("skills %q: %v", ok, err)
		}
	}
	for _, bad := range []string{"latest", "v0.16.0", "0.16", "none", "0.16.0 "} {
		if err := checkVersion(VersionInput{Skills: bad}); err == nil {
			t.Errorf("skills %q: no error", bad)
		}
	}
}
