package publish

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/publish/workdir"
	"github.com/holistics/anfra/internal/repoconfig"
	"github.com/holistics/anfra/shared/jsonkit"
	shared "github.com/holistics/anfra/shared/repoconfig"
	"github.com/holistics/anfra/shared/repoformat"
)

// orgSettings are the org's repo settings, as its server says they apply
// (repo_config.get), or that its server predates them.
type orgSettings struct {
	// supported is a server that publishes the repo's settings file: pull and
	// publish carry it, with with_config. False, they leave it out, as an
	// older anfra did.
	//
	// TEMPORARY (anfra-cloud's docs/implementation/repo-config.md, Temporary
	// checks): every supported server takes the file once old ones are gone.
	supported bool
	config    cloudapi.RepoConfigOut
}

// loadOrgSettings reads the org's repo settings, once per command, before
// anything compares or sends the folder's files.
func loadOrgSettings(ctx context.Context, client *cloudapi.Client) (orgSettings, error) {
	out, ok, err := client.RepoConfig(ctx)
	if err != nil {
		return orgSettings{}, err
	}
	return orgSettings{supported: ok, config: out}, nil
}

// scope is tree as the org's server takes it: without the settings file for
// a server that predates it, since that server refuses the file.
func (s orgSettings) scope(tree workdir.Tree) workdir.Tree {
	if s.supported {
		return tree
	}
	return tree.Without(repoformat.ConfigFile)
}

// warnUnsupported warns, once, that the org's server doesn't take the
// settings file, when the folder has one.
func (s orgSettings) warnUnsupported(t *told, tree workdir.Tree, org cloudapi.Org) {
	if s.supported {
		return
	}
	if _, ok := tree.Files()[repoformat.ConfigFile]; ok {
		t.Warn("settings_not_published", "%s doesn't take the repo's settings yet, so %s stays on this machine. Its other files are published as usual.", org, repoformat.ConfigFile)
	}
}

// overriddenChanges are the changes this publish makes to settings the org
// overrides, as warnings: each takes no effect on the org while the override
// stays. A change is a setting whose value in the folder's settings file,
// which tree says the publish sends, differs from the published file's.
func (s orgSettings) overriddenChanges(dir string, tree workdir.Tree, org cloudapi.Org) []string {
	if !s.supported {
		return nil
	}
	mine := repoconfig.Config{}
	if _, sent := tree.Files()[repoformat.ConfigFile]; sent {
		var err error
		if mine, err = repoconfig.ReadShared(dir); err != nil {
			return nil // validation says why
		}
	}
	var out []string
	for _, st := range s.config.Settings {
		if st.Source != "override" || st.Override == nil {
			continue
		}
		k := shared.Key(st.Key)
		var published []byte
		if st.Repo != nil {
			published, _ = jsonkit.Marshal(st.Repo)
		}
		if shared.Equal(k, mine.Get(k), published) {
			continue
		}
		by := "an Admin"
		if st.Override.UpdatedBy != "" {
			by = st.Override.UpdatedBy
		}
		out = append(out, fmt.Sprintf("This publish changes %s, but %s keeps it at %s, because %s overrides it there. The change takes effect there once the override is removed.",
			k, org, shown(st.Value), by))
	}
	return out
}

// localDifferences are the settings .anfra/config.local.yml sets on this
// machine that the org runs with another value, as warnings.
func (s orgSettings) localDifferences(dir string) []string {
	if !s.supported {
		return nil
	}
	keys, err := repoconfig.LocalKeys(dir)
	if err != nil || len(keys) == 0 {
		return nil
	}
	local, err := repoconfig.Read(dir)
	if err != nil {
		return nil // validation says why
	}
	inEffect := map[string]any{}
	for _, st := range s.config.Settings {
		inEffect[st.Key] = st.Value
	}
	var out []string
	for _, k := range keys {
		mine, theirs := local.Get(k), inEffect[k.String()]
		var theirsJSON []byte
		if theirs != nil {
			theirsJSON, _ = jsonkit.Marshal(theirs)
		}
		if shared.Equal(k, mine, theirsJSON) {
			continue
		}
		out = append(out, fmt.Sprintf("%s sets %s to %s on this machine, but the org uses %s.", shared.LocalFilePath, k, shownOrDefault(decode(mine)), shownOrDefault(theirs)))
	}
	return out
}

// shown is a setting's value as a message shows it: a string bare, anything
// else as JSON.
func shown(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	b, err := jsonkit.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// shownOrDefault is shown, or "anfra's default" for a value absent.
func shownOrDefault(v any) string {
	if v == nil {
		return "anfra's default"
	}
	return shown(v)
}

// decode is a setting's JSON value as Go's: nil for none.
func decode(v []byte) any {
	if v == nil {
		return nil
	}
	var out any
	if err := jsonkit.Unmarshal(v, &out); err != nil {
		return string(v)
	}
	return out
}

// adoptOrgConfig takes the org's settings file into a folder that has no
// base, when the folder's own sets nothing but anfra's defaults and org.url:
// the file `anfra init` writes. The merge would otherwise mark the two as a
// conflict, though the folder's has nothing to keep. The file is the org's
// exactly, so the merge finds the two the same. orgURL is the folder's
// org.url, for the caller to set again once the merge is done (keepOrgURL). A
// folder whose file sets anything else is left to the merge.
func adoptOrgConfig(ctx context.Context, client *cloudapi.Client, dir, revision string) (orgURL string, err error) {
	path := repoconfig.Path(dir)
	raw, err := os.ReadFile(path) //nolint:gosec // G304: the settings file of the repo being pulled into
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil // the merge adds the org's
	}
	if err != nil {
		return "", err
	}
	mine, err := shared.Parse(repoconfig.RelPath, raw)
	if err != nil || len(shared.Diff(shared.Config{}, mine)) > 0 {
		return "", nil // validation says what's wrong, or the merge decides
	}
	files, err := client.ListFiles(ctx, revision, true)
	if err != nil {
		return "", err
	}
	oid := ""
	for _, f := range files {
		if f.Path == repoformat.ConfigFile {
			oid = f.OID
		}
	}
	if oid == "" || oid == repoformat.BlobID(raw) {
		return "", nil
	}
	got, err := client.GetBlobs(ctx, []string{oid})
	if err != nil {
		return "", err
	}
	if len(got.Blobs) != 1 {
		return "", fmt.Errorf("read %s from the org: %d blobs answered for one", repoformat.ConfigFile, len(got.Blobs))
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, got.Blobs[0].Content, info.Mode().Perm()); err != nil {
		return "", err
	}
	return mine.Org.URL, nil
}

// keepOrgURL sets the folder's org.url back to url after adoptOrgConfig took
// the org's file, unless the file says it already. A change of the folder's
// own, as `anfra publish --set-org` makes.
func keepOrgURL(dir, url string) error {
	if url == "" {
		return nil
	}
	c, err := repoconfig.ReadShared(dir)
	if err != nil || c.Org.URL == url {
		return err
	}
	return repoconfig.SetString(repoconfig.Path(dir), "org", "url", url)
}
