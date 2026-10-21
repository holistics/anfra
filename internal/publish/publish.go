// Package publish is `anfra publish`: a local repo's files published to its
// org, as PUBLISH.md (anfra-cloud) describes. The command wires it to the
// terminal (Env); its messages name the command's flags.
package publish

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/repoconfig"
	"github.com/holistics/anfra/shared/repoformat"
)

// Options are a publish's choices: the command's flags.
type Options struct {
	// Message is what the publish is, for the org's history of the repo; "" is
	// HEAD's commit, in a git repo.
	Message string
	// SetOrg is an org's address to record as the repo's, and publish to.
	SetOrg string
	// DryRun lists the files and stops.
	DryRun bool
	// Yes publishes without asking, though the repo was last published elsewhere.
	Yes bool
	// CreateDataSources adds the data sources the repo uses and the org lacks
	// without asking.
	CreateDataSources bool
}

// Env is what a publish needs from the command around it.
type Env struct {
	Out, ErrOut io.Writer
	Ask         Prompt
	// Validate validates the repo here, before anything is sent: showing what
	// fails on its writer, and an error refusing the publish. Always, and again on
	// the server, whose verdict on exactly the published files is the one that
	// publishes (PUBLISH.md §Decisions).
	Validate func(context.Context, io.Writer) error
	// Credential checks the selected login before destination configuration is
	// saved, returning an authorizer that resolves credentials for every request.
	Credential func(context.Context, cloudapi.Org) (func(*http.Request) error, error)
	// SelectOrg explicitly selects a first destination; nil means no interactive terminal.
	SelectOrg func(context.Context) (cloudapi.Org, error)
	// RecoverOrg offers interactive recovery after credential loading fails.
	// It must return the original error when recovery does not apply, and confirm
	// any destination change before returning a different organization.
	RecoverOrg func(context.Context, cloudapi.Org, error) (cloudapi.Org, error)
	// UserAgent names the CLI and its version, for the server.
	UserAgent string
	// MotherDuck is the person's MotherDuck account, to copy a DuckDB data source
	// to; nil offers no copy.
	MotherDuck MotherDuck
	// Input asks for a line of text, as Ask asks a question; nil with no
	// terminal.
	Input Input
}

// Run publishes the repo at dir: PUBLISH.md's flow, from file selection to the
// server's answer. Progress goes to env.ErrOut; the outcome to env.Out.
func Run(ctx context.Context, dir string, o Options, env Env) error {
	ask := env.Ask
	p, result := cliui.NewPrinter(env.ErrOut), cliui.NewPrinter(env.Out)
	org, configured, err := readOrg(dir)
	if o.SetOrg != "" {
		org, err = cloudapi.ParseOrg(o.SetOrg)
		configured = true
	}
	if err != nil {
		return err
	}
	files, leftOut, err := selectFiles(dir)
	if err != nil {
		return err
	}
	selected := fmt.Sprintf("%d files of the repo", len(files))
	noteLeftOut := func() {
		if leftOut > 0 {
			p.Note("%d other files in this folder are not part of the repo: a publish carries .aml files and Data Apps under %s/.", leftOut, repoformat.AppsDir)
		}
	}
	if o.DryRun {
		for _, f := range files {
			fmt.Fprintln(env.Out, f)
		}
		if configured {
			p.Step("Would publish %s to %s.", selected, org)
		} else {
			p.Step("Would publish %s (no org configured).", selected)
		}
		noteLeftOut()
		return nil
	}
	if len(files) == 0 {
		return errors.New("there are no files to publish")
	}
	if !configured {
		if env.SelectOrg == nil {
			return errors.New("this repo has no org to publish to: set org.url or pass --set-org https://<server>/o/<org>; destination selection requires an interactive terminal")
		}
		org, err = env.SelectOrg(ctx)
		if err != nil {
			return err
		}
		// Selection is supplied by the command; keep the same address validation
		// as configured destinations before selecting credentials or writing config.
		org, err = cloudapi.ParseOrg(org.String())
		if err != nil {
			return err
		}
	}
	authorize, err := env.Credential(ctx, org)
	destinationChanged := false
	if err != nil && configured && env.RecoverOrg != nil {
		recovered, recoverErr := env.RecoverOrg(ctx, org, err)
		if recoverErr != nil {
			return recoverErr
		}
		recovered, err = cloudapi.ParseOrg(recovered.String())
		if err != nil {
			return err
		}
		destinationChanged = recovered != org
		org = recovered
		authorize, err = env.Credential(ctx, org)
	}
	if err != nil {
		return err
	}
	if !configured || o.SetOrg != "" || destinationChanged {
		if err := setOrg(dir, org); err != nil {
			return err
		}
		p.Success("This repo publishes to %s (%s).", org, repoconfig.Hint("org", "url"))
	}
	client := cloudapi.NewClient(org, cloudapi.ClientOptions{Authorize: authorize, UserAgent: env.UserAgent})

	p.Step("Validating…")
	if err := env.Validate(ctx, env.ErrOut); err != nil {
		return err
	}
	if err := offerDataSourcesToOrg(ctx, &mdCopy{md: env.MotherDuck, c: client, org: org, p: p, ask: ask, input: env.Input}, dir, o.CreateDataSources); err != nil {
		return err
	}
	warnOfOriginsOrgLacks(ctx, client, p, dir)

	oids := make(map[string]string, len(files)) // path → blob id
	sizes := make(map[string]int, len(files))
	for _, f := range files {
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f)))
		if err != nil {
			return err
		}
		oids[f], sizes[f] = repoformat.BlobID(content), len(content)
	}

	status, err := client.Status(ctx)
	if err != nil {
		return err
	}
	limits := status.Publish.Limits
	if len(files) > limits.Files {
		return fmt.Errorf("the repo has %d files to publish; %s takes at most %d", len(files), org, limits.Files)
	}
	for _, f := range files {
		if sizes[f] > limits.UploadBytes {
			return fmt.Errorf("%s is %d bytes; %s takes files of at most %d", f, sizes[f], org, limits.UploadBytes)
		}
	}
	// What was published since this folder last published is replaced, unseen:
	// there is no pull. So it is said, and confirmed (PUBLISH.md §anfra publish).
	state := publishState(dir)
	if status.PublishedBy != nil && state.last(org) != status.Revision {
		// TODO(OAUTH.md): "you" for the signed-in user's own publishes, once the
		// CLI knows who it is.
		p.Gap()
		p.Warn("This repo was last published by %s on %s: publishing replaces that version.",
			who(status.PublishedBy), status.PublishedAt.Local().Format(time.DateTime))
		if !o.Yes {
			yes, ok := Confirm(ask, "Confirm publishing a new version?")
			if !ok {
				return errors.New("not published: there is no one to confirm it here; pass --yes to publish")
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if !yes {
				return errors.New("not published")
			}
		}
	}

	byOID := map[string]string{} // blob id → a path holding it
	for _, f := range files {
		byOID[oids[f]] = f
	}
	missing, err := listMissing(ctx, client, slices.Sorted(maps.Keys(byOID)), limits.Files)
	if err != nil {
		return err
	}
	p.Gap()
	up := uploader{client: client, dir: dir, byOID: byOID, sizes: sizes, limits: limits, progress: p}
	if err := up.upload(ctx, missing); err != nil {
		return err
	}

	in := cloudapi.PublishIn{Message: cmp.Or(o.Message, gitMessage(ctx, dir))}
	for _, f := range files {
		in.Files = append(in.Files, cloudapi.File{Path: f, OID: oids[f]})
	}
	p.Step("Publishing %s to %s…", selected, org)
	noteLeftOut()
	res, err := client.Publish(ctx, in)
	if again, ok := cloudapi.Missing(err); ok {
		// Uploaded, then collected before this publish named it: once more.
		if err := up.upload(ctx, again); err != nil {
			return err
		}
		res, err = client.Publish(ctx, in)
	}
	if err != nil {
		return err
	}
	state.record(org, res.Revision)
	if !res.Changed {
		p.Gap()
		result.Success("Nothing changed: %s is already published.", res.Revision)
		return nil
	}
	p.Gap()
	result.Success("Published %s.", res.Revision)
	if r := res.Replaced; r != nil {
		result.Note("It replaced %s, published by %s on %s.", r.Revision, who(r.PublishedBy), r.PublishedAt.Local().Format(time.DateTime))
	}
	return nil
}

// listMissing asks which of oids the org's repo lacks, in calls of at most
// perCall.
func listMissing(ctx context.Context, c *cloudapi.Client, oids []string, perCall int) ([]string, error) {
	var missing []string
	for batch := range slices.Chunk(oids, max(perCall, 1)) {
		m, err := c.ListMissingBlobs(ctx, batch)
		if err != nil {
			return nil, err
		}
		missing = append(missing, m...)
	}
	return missing, nil
}

// uploader uploads a publish's content to the org's repo.
type uploader struct {
	client   *cloudapi.Client
	dir      string
	byOID    map[string]string // blob id → a file holding it
	sizes    map[string]int    // file → its size
	limits   cloudapi.Limits
	progress *cliui.Printer
}

// upload uploads the blobs oids, reading each from a file holding it, in calls
// within the server's limits.
func (u uploader) upload(ctx context.Context, oids []string) error {
	if len(oids) == 0 {
		return nil
	}
	c, dir, byOID, limits := u.client, u.dir, u.byOID, u.limits
	total := 0
	for _, oid := range oids {
		total += u.sizes[byOID[oid]]
	}
	u.progress.Step("Uploading %d files (%s)…", len(oids), byteSize(total))
	var batch [][]byte
	var batchOIDs []string
	size := 0
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		got, err := c.CreateBlobs(ctx, batch)
		if err != nil {
			return err
		}
		if !slices.Equal(got, batchOIDs) {
			return fmt.Errorf("the server named the uploaded content %v, not %v: it and this anfra hash content differently", got, batchOIDs)
		}
		batch, batchOIDs, size = nil, nil, 0
		return nil
	}
	for _, oid := range oids {
		path := byOID[oid]
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
		if err != nil {
			return err
		}
		if repoformat.BlobID(content) != oid {
			return fmt.Errorf("%s changed while publishing: publish again", path)
		}
		if len(batch) == limits.UploadBlobs || size+len(content) > limits.UploadBytes {
			if err := flush(); err != nil {
				return err
			}
		}
		batch, batchOIDs, size = append(batch, content), append(batchOIDs, oid), size+len(content)
	}
	return flush()
}

// gitMessage is a publish's default message: in a git repo, HEAD's short id and
// subject, marked when the work tree has changes not in it; "" otherwise.
func gitMessage(ctx context.Context, dir string) string {
	head, err := exec.CommandContext(ctx, "git", "-C", dir, "log", "-1", "--format=%h %s").Output()
	if err != nil {
		return ""
	}
	msg := strings.TrimSpace(string(head))
	if dirty, err := exec.CommandContext(ctx, "git", "-C", dir, "status", "--porcelain", "--", ".").Output(); err == nil && len(bytes.TrimSpace(dirty)) > 0 {
		msg += " (with uncommitted changes)"
	}
	return msg
}

func who(p *cloudapi.Person) string {
	switch {
	case p == nil:
		return "nobody (the repo's starting files)"
	case p.Email == "":
		return "a former member"
	}
	return p.Email
}

func byteSize(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}
