package publish

import (
	"context"
	"slices"
	"strings"

	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/repoconfig"
)

// warnOfOriginsOrgLacks says which origins this repo lets its Data Apps load from
// (.anfra/config.yml apps.allow_origins) the org does not: the repo's list is
// never published, as the org keeps its own, which an admin edits in Organization
// settings. Apps that load from one work here and are blocked there. Nothing here
// stops the publish, and nothing is said when the repo lists none.
func warnOfOriginsOrgLacks(ctx context.Context, c *cloudapi.Client, p *cliui.Printer, dir string) {
	conf, err := repoconfig.Read(dir)
	if err != nil {
		return // validation has said so
	}
	local, err := conf.AllowedOrigins()
	if err != nil {
		p.Warn("Not checking the org's allowed origins: %v", err)
		return
	}
	if len(local) == 0 {
		return
	}
	p.Section("Data Apps")
	defer p.Section("")
	have, err := c.ListAppOrigins(ctx)
	if err != nil {
		p.Warn("Not checking the org's allowed origins: %v", err)
		return
	}
	var lacking []string
	for _, o := range local {
		if !slices.Contains(have, o) && !slices.Contains(lacking, o) {
			lacking = append(lacking, o)
		}
	}
	if len(lacking) == 0 {
		return
	}
	it := "it"
	if len(lacking) > 1 {
		it = "them"
	}
	p.Warn("The org does not allow %s, which this repo's Data Apps may load from (%s): apps loading from %s are blocked there.",
		strings.Join(lacking, ", "), repoconfig.Hint("apps", "allow_origins"), it)
	p.Note("An org admin adds origins in Organization settings → Data Apps.")
}
