package agents

import (
	"strings"

	"github.com/holistics/anfra/shared/jsonkit"
)

// claudeInstalled reads our plugin's copies from `claude plugin list --json`:
// an array of {id, version, scope, …}, one entry per scope it is installed at.
func claudeInstalled(listJSON []byte) []Installed {
	var plugins []struct {
		ID      string `json:"id"`
		Version string `json:"version"`
		Scope   string `json:"scope"`
	}
	if jsonkit.Unmarshal(listJSON, &plugins) != nil {
		return nil
	}
	var out []Installed
	for _, p := range plugins {
		if strings.EqualFold(p.ID, PluginID) {
			out = append(out, Installed{Version: p.Version, Scope: p.Scope})
		}
	}
	return out
}

// codexInstalled reads our plugin from `codex plugin list --marketplace … --json`:
// {"installed": [{"pluginId", "name", "marketplaceName", "version"}, …]}, the
// shape Stripe's CLI reads from codex-cli 0.142.0. Codex installs per user only.
func codexInstalled(listJSON []byte) []Installed {
	var list struct {
		Installed []struct {
			PluginID    string `json:"pluginId"`
			Name        string `json:"name"`
			Marketplace string `json:"marketplaceName"`
			Version     string `json:"version"`
		} `json:"installed"`
	}
	if jsonkit.Unmarshal(listJSON, &list) != nil {
		return nil
	}
	for _, p := range list.Installed {
		if strings.EqualFold(p.PluginID, PluginID) || strings.EqualFold(p.Name, Plugin) && strings.EqualFold(p.Marketplace, Marketplace) {
			return []Installed{{Version: p.Version, Scope: "user"}}
		}
	}
	return nil
}
