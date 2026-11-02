package agents

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/holistics/anfra/internal/home"
	"github.com/holistics/anfra/internal/update"
	"github.com/holistics/anfra/shared/jsonkit"
)

// noticeEvery is how often, at most, the notice after a command prints.
const noticeEvery = 24 * time.Hour

// state is what anfra keeps between runs about the agents: the plugins the
// agents' CLIs last reported, which the notice reads without starting them,
// and when the notice last printed.
type state struct {
	Plugins   map[string][]Installed `json:"plugins,omitempty"`
	CheckedAt time.Time              `json:"checked_at,omitzero"`
	NoticedAt time.Time              `json:"noticed_at,omitzero"`
}

func statePath() string { return filepath.Join(home.Dir(), "agents-check.json") }

func readState() state {
	var s state
	if data, err := os.ReadFile(statePath()); err == nil {
		_ = jsonkit.Unmarshal(data, &s)
	}
	return s
}

func writeState(s state) {
	p := statePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return
	}
	if data, err := jsonkit.Marshal(s); err == nil {
		_ = os.WriteFile(p, data, 0o600)
	}
}

// RecordPlugins asks each agent's CLI for its plugins and keeps the answer,
// for the notice. Agents' CLIs start slowly, so this is for the background
// check and setup, never an ordinary command.
func RecordPlugins(ctx context.Context, env Env) {
	s := readState()
	s.Plugins = map[string][]Installed{}
	for _, a := range Known {
		if got := a.InstalledPlugin(ctx, env); len(got) > 0 {
			s.Plugins[a.ID] = got
		}
	}
	s.CheckedAt = time.Now()
	writeState(s)
}

// Attention is one line saying what of the agents' skills needs attention,
// and to run setup: the plugins behind, as anfra last recorded them; the
// copies behind and with deprecated skills, in the folders the agents read;
// and repoDir's AGENTS.md block ("" outside a repo). "" when nothing does. No
// network and no agent CLI: it is cheap enough to run after a command.
func Attention(env Env, repoDir string) string {
	latest, _, known := update.CachedSkills()
	var behind, others []string
	if known {
		plugins := readState().Plugins
		for _, a := range Known {
			for _, p := range plugins[a.ID] {
				if Newer(latest, p.Version) {
					behind = append(behind, "the "+a.Name+" plugin")
					break
				}
			}
		}
	}
	for _, c := range Copies(env, repoDir) {
		if len(c.Versions) > 0 && known {
			if st, _ := status(c.Versions, latest, known); st == "behind" || st == "mixed" {
				behind = append(behind, c.Dir)
			}
		}
		if c.OldNames {
			others = append(others, c.Dir+" has deprecated anfra skills")
		}
	}
	if repoDir != "" {
		if b, err := CheckBlock(repoDir); err == nil {
			switch {
			case b.State == BlockMissing:
				others = append(others, "this repo's AGENTS.md has no anfra block")
			case b.State == BlockOutdated:
				others = append(others, "this repo's AGENTS.md block is out of date")
			case b.NoImport:
				others = append(others, "this repo's CLAUDE.md doesn't import its AGENTS.md")
			}
		}
	}

	var line string
	more := len(others)
	switch {
	case len(behind) > 0:
		shown := behind[:min(2, len(behind))]
		more += len(behind) - len(shown)
		verb := "is"
		if len(shown) > 1 {
			verb = "are"
		}
		line = fmt.Sprintf("anfra skills %s are out: %s %s behind", latest, strings.Join(shown, " and "), verb)
	case len(others) > 0:
		line = strings.ToUpper(others[0][:1]) + others[0][1:]
		more--
	case noSkills(env, repoDir):
		return "Your coding agents have no anfra skills. Run `anfra agent setup` to install them."
	default:
		return ""
	}
	switch {
	case more == 1:
		line += ", and 1 more thing needs attention"
	case more > 1:
		line += fmt.Sprintf(", and %d more things need attention", more)
	}
	return line + ". Run `anfra agent setup`."
}

// noSkills reports whether the agents on this machine have no anfra skills: a
// coding agent is detected, the plugins have been recorded at least once and
// none is installed, and no folder the agents read has any. Without an agent,
// or before the plugins are known, it can't tell, and says false.
func noSkills(env Env, repoDir string) bool {
	s := readState()
	if s.CheckedAt.IsZero() || len(s.Plugins) > 0 || len(Copies(env, repoDir)) > 0 {
		return false
	}
	for _, a := range Known {
		if a.Detected(env) {
			return true
		}
	}
	return false
}

// Notice is Attention, at most once a day: "" when it printed less than a day
// ago, or nothing needs attention. It records that it printed.
func Notice(env Env, repoDir string) string {
	s := readState()
	if time.Since(s.NoticedAt) < noticeEvery {
		return ""
	}
	line := Attention(env, repoDir)
	if line != "" {
		s.NoticedAt = time.Now()
		writeState(s)
	}
	return line
}
