package publish

import (
	"os"
	"path/filepath"

	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/shared/jsonkit"
)

// publishedState is what this folder last published to each org, in the repo's
// state folder: whether the org's published revision is one published from
// here.
type publishedState struct{ path string }

func publishState(dir string) publishedState {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return publishedState{path: filepath.Join(repo.Resolve(abs).DataDir, "published.json")}
}

func (s publishedState) read() map[string]string {
	m := map[string]string{}
	if raw, err := os.ReadFile(s.path); err == nil {
		_ = jsonkit.Unmarshal(raw, &m)
	}
	return m
}

func (s publishedState) last(org cloudapi.Org) string { return s.read()[org.String()] }

// record notes revision as published from here. Best-effort: without it, the
// next publish asks before replacing what it cannot tell was its own.
func (s publishedState) record(org cloudapi.Org, revision string) {
	m := s.read()
	m[org.String()] = revision
	raw, err := jsonkit.Marshal(m)
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(s.path), 0o755) == nil { //nolint:gosec // G301: anfra's own state folder
		_ = os.WriteFile(s.path, raw, 0o600)
	}
}
