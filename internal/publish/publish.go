// Package publish is `anfra publish`: a local repo's files published to its
// org, as PUBLISH.md (anfra-cloud) describes. The command wires it to the
// terminal (Env); its messages name the command's flags.
package publish

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/publish/syncstate"
	"github.com/holistics/anfra/internal/repoconfig"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/repoformat"
)

// Options are a publish's choices: the command's flags.
type Options struct {
	// Message is what the publish is, for the org's history of the repo; "" is
	// HEAD's commit, in a git repo.
	Message string
	// SetOrg is an org's address to record as the repo's, and publish to.
	SetOrg string
	// Check is publish check: the preflight, which goes as far as a publish
	// would before its first change, then says what it would do. Today, the
	// part done here, offline, without authentication or config writes: the
	// files, and where they'd go.
	Check bool
	// Yes did nothing since publish stopped asking before replacing a
	// version; kept for a release, with a warning.
	Yes bool
	// CreateDataSources adds the data sources the repo uses and the org lacks
	// without asking.
	CreateDataSources bool
}

// Env is what a publish needs from the command around it.
type Env struct {
	// ErrOut is where progress and questions go, for a person at a terminal.
	ErrOut io.Writer
	// Warn says a problem that doesn't stop the publish, by its code; nil says
	// nothing.
	Warn func(code, message string)
	// OrgName is an org's name, as its login knows it; nil, or "": unknown.
	OrgName func(cloudapi.Org) string
	Ask     Prompt
	// Validate validates the repo here, before anything is sent: an error
	// refusing the publish, carrying what fails. Always, and again on the
	// server, whose verdict on exactly the published files is the one that
	// publishes (PUBLISH.md §Decisions).
	Validate func(context.Context) error
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
	// Logger is the repo's log, for what the publish absorbed that no one
	// sees, such as a retry; nil records nothing.
	Logger *slog.Logger
}

// Run publishes the repo at dir: PUBLISH.md's flow, from file selection to the
// server's answer. With o.Check it checks instead: a CheckResult; else a
// Result. Progress and questions go to env.ErrOut; warnings to env.Warn.
func Run(ctx context.Context, dir string, o Options, env Env) (any, error) {
	t := &told{warn: env.Warn}
	org, configured, err := readOrg(dir)
	if o.SetOrg != "" {
		org, err = cloudapi.ParseOrg(o.SetOrg)
		configured = true
	}
	if err != nil {
		return nil, err
	}
	files, err := selectFiles(dir)
	if err != nil {
		return nil, err
	}
	tg := target{files: files, org: org, configured: configured}
	if o.Check {
		if configured {
			// The one refusal known offline: the last pull isn't finished.
			state, err := syncstate.For(dir).Load()
			if err != nil {
				return nil, err
			}
			if _, err := pendingRefusal(dir, state[org.String()], verbCheck); err != nil {
				return nil, err
			}
		}
		return check(tg), nil
	}
	return publish(ctx, dir, o, env, t, tg)
}

// target is what a publish sends, and where: its org, configured, or to
// choose.
type target struct {
	files      []string
	org        cloudapi.Org
	configured bool
}

// check is what a publish would send, and where.
func check(tg target) CheckResult {
	files, org, configured := tg.files, tg.org, tg.configured
	res := CheckResult{Files: files}
	if !configured {
		res.Message = fmt.Sprintf("Would publish %d files. No org is configured.", len(files))
		res.Hints = []string{"Set the org with `anfra publish --set-org https://<server>/o/<org>`, or run `anfra publish` on a terminal to choose one."}
		return res
	}
	res.Org = &OrgRef{URL: org.String()}
	res.Message = fmt.Sprintf("Would publish %d files to %s.", len(files), org)
	return res
}

// publish publishes tg's files to its org, configured or chosen here.
func publish(ctx context.Context, dir string, o Options, env Env, t *told, tg target) (Result, error) {
	files, org, configured := tg.files, tg.org, tg.configured
	ask := env.Ask
	p := cliui.NewPrinter(env.ErrOut)
	selected := fmt.Sprintf("%d files of the repo", len(files))
	if len(files) == 0 {
		return Result{}, clierr.Precondition("Not published, because this folder has no .aml files, and no Data Apps under %s/. Run `anfra publish` in the repo's folder.", repoformat.AppsDir)
	}
	if o.Yes {
		t.Warn("yes_ignored", "`--yes` is deprecated and does nothing now, so stop passing it. Publish no longer asks before replacing someone else's work. Instead, it stops, and asks you to run `anfra pull` first.")
	}
	sc, release, err := openSync(dir, p)
	if err != nil {
		return Result{}, err
	}
	defer release()
	org, client, err := connect(ctx, dir, destination{org, configured, o.SetOrg != ""}, env, p, verbPublish)
	if err != nil {
		return Result{}, err
	}
	if err := settle(dir, sc.sync, sc.state, org, verbPublish); err != nil {
		return Result{}, err
	}

	p.Step("Validating…")
	if err := env.Validate(ctx); err != nil {
		return Result{}, err
	}

	oids := make(map[string]string, len(files)) // path → blob id
	sizes := make(map[string]int, len(files))
	for _, f := range files {
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f)))
		if err != nil {
			return Result{}, err
		}
		oids[f], sizes[f] = repoformat.BlobID(content), len(content)
	}

	status, err := client.Status(ctx)
	if err != nil {
		return Result{}, err
	}
	// The org's limits are checked before anything is changed, in MotherDuck
	// or on the org.
	limits := status.Limits
	if len(files) > limits.Files {
		return Result{}, clierr.Precondition("Not published, because the repo has %d files to publish, and %s takes at most %d.", len(files), org, limits.Files)
	}
	for _, f := range files {
		if sizes[f] > limits.MaxFileBytes() {
			return Result{}, clierr.Precondition("Not published, because %s is %s, and %s takes files of at most %s.", f, byteSize(sizes[f]), org, byteSize(limits.MaxFileBytes()))
		}
	}
	if err := offerDataSourcesToOrg(ctx, &mdCopy{md: env.MotherDuck, c: client, org: org, p: p, t: t, ask: ask, input: env.Input}, dir, o.CreateDataSources); err != nil {
		return Result{}, err
	}
	warnOfOriginsOrgLacks(ctx, client, t, p, dir)
	// What the org has that this folder hasn't seen is never replaced unseen
	// (docs/designs/publish.md, flow 6).
	if err := sc.catchUp(ctx, client, org, status, oids); err != nil {
		return Result{}, err
	}

	byOID := map[string]string{} // blob id → a path holding it
	for _, f := range files {
		byOID[oids[f]] = f
	}
	missing, err := listMissing(ctx, client, slices.Sorted(maps.Keys(byOID)), limits.Files)
	if err != nil {
		return Result{}, err
	}
	p.Gap()
	up := uploader{client: client, dir: dir, byOID: byOID, sizes: sizes, limits: limits, progress: p}
	if err := up.upload(ctx, missing); err != nil {
		return Result{}, err
	}

	in := cloudapi.PublishIn{Message: cmp.Or(o.Message, gitMessage(ctx, dir))}
	for _, f := range files {
		in.Files = append(in.Files, cloudapi.File{Path: f, OID: oids[f]})
	}
	p.Step("Publishing %s to %s…", selected, org)
	in.Base = sc.base
	res, err := client.Publish(ctx, in)
	if again, ok := cloudapi.Missing(err); ok {
		// Uploaded, then collected before this publish named it: once more.
		if err := up.upload(ctx, again); err != nil {
			return Result{}, err
		}
		res, err = client.Publish(ctx, in)
	}
	if live, ok := cloudapi.Outdated(err); ok {
		// Someone published between the check and this publish.
		return Result{}, pullRequired(org, live, true)
	}
	if err != nil {
		return Result{}, err
	}
	sc.record(org, res.Revision)

	ref := OrgRef{URL: org.String()}
	if env.OrgName != nil {
		ref.Name = env.OrgName(org)
	}
	shown := cmp.Or(ref.Name, ref.URL)
	out := Result{Org: ref, Revision: res.Revision, Changed: res.Changed, Replaced: res.Replaced, Files: len(files),
		DataSourcesCreated: append([]string{}, t.created...)}
	if res.Changed {
		out.Message = fmt.Sprintf("Published %d files to %s, as revision %s.", len(files), shown, res.Revision)
	} else {
		out.Message = fmt.Sprintf("Nothing changed: %s is already published to %s.", res.Revision, shown)
	}
	t.Hint("Open it at %s.", org)
	out.Hints = t.hints
	return out, nil
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
			return clierr.Precondition("Not published, because %s changed during the publish. Publish again.", path)
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

// verb is how a command's messages name it: "Not published", or "Not
// pulled".
type verb struct{ not, command, toOrg string }

var (
	verbPublish = verb{not: "Not published", command: "anfra publish", toOrg: "to publish to"}
	verbPull    = verb{not: "Not pulled", command: "anfra pull", toOrg: "to pull from"}
	verbCheck   = verb{not: "Wouldn't publish", command: "anfra publish", toOrg: "to publish to"}
)

// destination is the org a command acts on, as the repo or its flags give it.
type destination struct {
	org         cloudapi.Org
	configured  bool // the repo's org, or one --set-org gave
	setOrgGiven bool // given by --set-org: saved as the repo's
}

// connect is a command's org and a client of it: the org configured, or one
// chosen at a terminal, its login checked, recovered at a terminal when it
// fails, and saved as the repo's org when it's new or changed.
func connect(ctx context.Context, dir string, d destination, env Env, p *cliui.Printer, v verb) (cloudapi.Org, *cloudapi.Client, error) {
	org, configured, setOrgGiven := d.org, d.configured, d.setOrgGiven
	var err error
	if !configured {
		if env.SelectOrg == nil {
			return org, nil, apperr.New(clierr.OrgNotSet, fmt.Sprintf("%s, because this repo has no org %s. Pass --set-org with the org's address (https://<server>/o/<org>), or run `%s` in a terminal to choose one.", v.not, v.toOrg, v.command))
		}
		org, err = env.SelectOrg(ctx)
		if err != nil {
			return org, nil, err
		}
		// Selection is supplied by the command; keep the same address validation
		// as configured destinations before selecting credentials or writing config.
		org, err = cloudapi.ParseOrg(org.String())
		if err != nil {
			return org, nil, err
		}
	}
	authorize, err := env.Credential(ctx, org)
	destinationChanged := false
	if err != nil && configured && env.RecoverOrg != nil {
		recovered, recoverErr := env.RecoverOrg(ctx, org, err)
		if recoverErr != nil {
			return org, nil, recoverErr
		}
		recovered, err = cloudapi.ParseOrg(recovered.String())
		if err != nil {
			return org, nil, err
		}
		destinationChanged = recovered != org
		org = recovered
		authorize, err = env.Credential(ctx, org)
	}
	if err != nil {
		return org, nil, err
	}
	if !configured || setOrgGiven || destinationChanged {
		if err := setOrg(dir, org); err != nil {
			return org, nil, err
		}
		p.Success("This repo publishes to %s (%s).", org, repoconfig.Hint("org", "url"))
	}
	client := cloudapi.NewClient(org, cloudapi.ClientOptions{
		Authorize: authorize,
		UserAgent: env.UserAgent,
		Logger:    env.Logger,
		Waiting: func(_ string, wait time.Duration) {
			p.Step("%s asked anfra to slow down. anfra will try again in %s…", org, wait.Round(time.Second))
		},
	})
	return org, client, nil
}
