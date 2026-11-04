package publish

import (
	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/repoconfig"
)

// readOrg is the org the repo at dir publishes to; ok is false when that is not
// set.
func readOrg(dir string) (org cloudapi.Org, ok bool, err error) {
	c, err := repoconfig.Read(dir)
	if err != nil || c.Org.URL == "" {
		return org, false, err
	}
	if org, err = cloudapi.ParseOrg(c.Org.URL); err != nil {
		return org, false, clierr.Precondition("%s: org.url: %v.", repoconfig.Path(dir), err)
	}
	return org, true, nil
}

// setOrg records org as the one the repo at dir publishes to.
func setOrg(dir string, org cloudapi.Org) error {
	return repoconfig.SetString(repoconfig.Path(dir), "org", "url", org.String())
}
