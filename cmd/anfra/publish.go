package main

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/holistics/anfra/internal/app"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/shared/jsonkit"
	"github.com/holistics/anfra/shared/remoteapi"
)

// sessionEnv carries a session token to publish with, until `anfra login`
// exists: for development only, against a server whose session is a cookie.
const sessionEnv = "ANFRA_REMOTE_SESSION"

// publishOptions are anfra publish's flags.
type publishOptions struct {
	message   string
	setRemote string
	dryRun    bool
	yes       bool
	// createDataSources adds the data sources the repo uses and the org lacks
	// without asking.
	createDataSources bool
}

func newPublishCmd() *cobra.Command {
	var o publishOptions
	cmd := &cobra.Command{
		Use:   "publish",
		Short: "Publish this repo to its org on an anfra remote server: its files, as they are now, become the published version",
		Long: "Publish this repo's files to its org on an anfra remote server, as they are in this folder now, committed\n" +
			"in git or not. They replace what is published there, all of it: a file not here is deleted there.\n" +
			"They are validated here first, then on the server, which publishes nothing that fails.\n\n" +
			"Which files: the repo's own, its .aml files and its Data Apps (everything under apps/), wherever they\n" +
			"are in the folder, except under a dot-folder or node_modules. Nothing else is published: not .anfra/\n" +
			"(data_sources.yml holds credentials), nor a .env, notes or build output beside the repo. .gitignore does\n" +
			"not apply: it says what belongs in git, not what is published. --dry-run lists the files and stops.\n\n" +
			"Where to: the org's address on the anfra remote server, https://<server>/o/<org>, kept in\n" +
			".anfra/config.yml (remote.url) and committed, so everyone publishes to the same org. --set-remote sets it,\n" +
			"and publishes there. The anfra remote server is not a git remote.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return publish(cmd.Context(), ".", o, cmd.OutOrStdout(), cmd.ErrOrStderr(), stdinPrompt(cmd.InOrStdin(), cmd.ErrOrStderr()))
		},
	}
	f := cmd.Flags()
	f.StringVarP(&o.message, "message", "m", "", "what this publish is, for the repo's history on the anfra remote server (default: in a git repo, HEAD's commit)")
	f.StringVar(&o.setRemote, "set-remote", "", "set where the repo publishes, an org's address on an anfra remote server, https://<server>/o/<org> (.anfra/config.yml remote.url), and publish there")
	f.BoolVar(&o.dryRun, "dry-run", false, "list the files that would be published, and stop")
	f.BoolVar(&o.createDataSources, "create-data-sources", false, "add to the org the data sources this repo uses (.anfra/data_sources.yml) and the org lacks, without asking (needed without a terminal); their connections are sent to the server")
	f.BoolVarP(&o.yes, "yes", "y", false, "publish without asking, though the repo was last published elsewhere (needed without a terminal)")
	return cmd
}

// prompt asks a question with choices, the last one the default (the safe
// answer), and returns the choice's index; ok is false when nobody can answer it
// (not a terminal).
type prompt func(question string, choices ...string) (choice int, ok bool)

// confirm asks a yes-or-no question, no the default.
func confirm(ask prompt, question string) (yes, ok bool) {
	choice, ok := ask(question, "yes", "no")
	return ok && choice == 0, ok
}

// stdinPrompt asks on out and reads answers from in: a choice's first letter or
// whole name, or nothing for the default. One reader for every question, so an
// answer read ahead is not lost to the next one.
func stdinPrompt(in io.Reader, out io.Writer) prompt {
	var lines *bufio.Reader
	return func(question string, choices ...string) (int, bool) {
		if f, isFile := in.(*os.File); isFile {
			if stat, err := f.Stat(); err != nil || stat.Mode()&os.ModeCharDevice == 0 {
				return 0, false
			}
		}
		if lines == nil {
			lines = bufio.NewReader(in)
		}
		fallback := len(choices) - 1
		labels := make([]string, len(choices))
		for i, c := range choices {
			labels[i] = "[" + c[:1] + "]" + c[1:]
		}
		fmt.Fprintf(out, "%s %s (default: %s) ", question, strings.Join(labels, " / "), choices[fallback])
		line, _ := lines.ReadString('\n')
		answer := strings.ToLower(strings.TrimSpace(line))
		for i, c := range choices {
			if answer != "" && (answer == c || answer == c[:1]) {
				return i, true
			}
		}
		return fallback, true
	}
}

// publish publishes the repo at dir: PUBLISH.md's flow, from file selection to
// the server's answer. Progress goes to errOut; the outcome to out.
func publish(ctx context.Context, dir string, o publishOptions, out, errOut io.Writer, ask prompt) error {
	remote, err := publishRemote(dir, o.setRemote, o.dryRun, errOut)
	if err != nil {
		return err
	}
	files, leftOut, err := selectFiles(dir)
	if err != nil {
		return err
	}
	selected := fmt.Sprintf("%d files of the repo", len(files))
	if leftOut > 0 {
		selected += fmt.Sprintf(" (%d other files in this folder are not part of it: a publish carries .aml files and Data Apps under %s/)", leftOut, remoteapi.AppsDir)
	}
	if o.dryRun {
		for _, f := range files {
			fmt.Fprintln(out, f)
		}
		fmt.Fprintf(errOut, "Would publish %s to %s.\n", selected, remote)
		return nil
	}
	if len(files) == 0 {
		return errors.New("there are no files to publish")
	}
	authorize, err := sessionCredential()
	if err != nil {
		return err
	}
	client := remoteapi.NewClient(remote, remoteapi.ClientOptions{Authorize: authorize, UserAgent: "anfra/" + meta.Version})

	fmt.Fprintln(errOut, "Validating…")
	if err := validateRepo(ctx, errOut); err != nil {
		return err
	}
	if err := offerDataSourcesToRemote(ctx, client, dir, o.createDataSources, errOut, ask); err != nil {
		return err
	}

	oids := make(map[string]string, len(files)) // path → blob id
	sizes := make(map[string]int, len(files))
	for _, f := range files {
		content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f)))
		if err != nil {
			return err
		}
		oids[f], sizes[f] = remoteapi.BlobID(content), len(content)
	}

	status, err := client.Status(ctx)
	if err != nil {
		return err
	}
	limits := status.Publish.Limits
	if len(files) > limits.Files {
		return fmt.Errorf("the repo has %d files to publish; %s takes at most %d", len(files), remote, limits.Files)
	}
	for _, f := range files {
		if sizes[f] > limits.UploadBytes {
			return fmt.Errorf("%s is %d bytes; %s takes files of at most %d", f, sizes[f], remote, limits.UploadBytes)
		}
	}
	// What was published since this folder last published is replaced, unseen:
	// there is no pull. So it is said, and confirmed (PUBLISH.md §anfra publish).
	state := publishState(dir)
	if status.PublishedBy != nil && state.last(remote) != status.Revision {
		// TODO(OAUTH.md): "you" for the signed-in user's own publishes, once the
		// CLI knows who it is.
		fmt.Fprintf(errOut, "This repo was last published by %s on %s.\n",
			who(status.PublishedBy), status.PublishedAt.Local().Format(time.DateTime))
		if !o.yes {
			yes, ok := confirm(ask, "Confirm publishing a new version?")
			if !ok {
				return errors.New("not published: there is no one to confirm it here; pass --yes to publish")
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
	up := uploader{client: client, dir: dir, byOID: byOID, sizes: sizes, limits: limits, progress: errOut}
	if err := up.upload(ctx, missing); err != nil {
		return err
	}

	in := remoteapi.PublishIn{Message: cmp.Or(o.message, gitMessage(ctx, dir))}
	for _, f := range files {
		in.Files = append(in.Files, remoteapi.File{Path: f, OID: oids[f]})
	}
	fmt.Fprintf(errOut, "Publishing %s to %s…\n", selected, remote)
	res, err := client.Publish(ctx, in)
	if again, ok := remoteapi.Missing(err); ok {
		// Uploaded, then collected before this publish named it: once more.
		if err := up.upload(ctx, again); err != nil {
			return err
		}
		res, err = client.Publish(ctx, in)
	}
	if err != nil {
		return err
	}
	state.record(remote, res.Revision)
	if !res.Changed {
		_, err = fmt.Fprintf(out, "Nothing changed: %s is already published.\n", res.Revision)
		return err
	}
	fmt.Fprintf(out, "Published %s.\n", res.Revision)
	if r := res.Replaced; r != nil {
		fmt.Fprintf(out, "It replaced %s, published by %s on %s.\n", r.Revision, who(r.PublishedBy), r.PublishedAt.Local().Format(time.DateTime))
	}
	return nil
}

// publishRemote is where a publish goes: the repo's remote, or set, which it
// records first (but not on a dry run, which changes nothing).
func publishRemote(dir, set string, dryRun bool, errOut io.Writer) (remoteapi.Remote, error) {
	if set == "" {
		r, ok, err := readRemote(dir)
		if err != nil {
			return remoteapi.Remote{}, err
		}
		if !ok {
			return remoteapi.Remote{}, fmt.Errorf("this repo has nowhere to publish to (%s): publish with --set-remote https://<server>/o/<org>, an org's address on an anfra remote server", configHint("remote", "url"))
		}
		return r, nil
	}
	r, err := remoteapi.ParseRemote(set)
	if err != nil {
		return remoteapi.Remote{}, err
	}
	if dryRun {
		return r, nil
	}
	if err := setRemote(dir, r); err != nil {
		return remoteapi.Remote{}, err
	}
	fmt.Fprintf(errOut, "This repo publishes to %s (%s).\n", r, configHint("remote", "url"))
	return r, nil
}

// sessionCredential is the credential a publish carries.
//
// TODO(OAUTH.md): `anfra login`, and its stored token. Until then, a session
// token from the environment, sent as the session cookie anfra-cloud's web app
// uses: for development only.
func sessionCredential() (func(*http.Request), error) {
	token := os.Getenv(sessionEnv)
	if token == "" {
		return nil, errors.New("not signed in: `anfra login` does not exist yet; for development, set " + sessionEnv + " to a session token")
	}
	return func(r *http.Request) { r.Header.Add("Cookie", "anfra_session="+token) }, nil
}

// validateRepo validates the repo before a publish: always, and again on the
// server, whose verdict on exactly the published files is the one that publishes
// (PUBLISH.md §Decisions). A variable for tests, which run no sidecars.
var validateRepo = validateHere

// validateHere runs validate on this repo, as `anfra validate` does; files that
// fail are shown, and refuse the publish.
func validateHere(ctx context.Context, errOut io.Writer) error {
	c, ok := app.Find("validate")
	if !ok {
		return errors.New("this anfra has no validate command")
	}
	body, err := invoke(ctx, c, map[string]any{})
	if err != nil {
		return err
	}
	if valid, err := c.Valid(body); err != nil || valid {
		return err
	}
	_ = renderTo(body, "application/json", errOut)
	return errors.New("the repo has errors: fix them and publish again")
}

// listMissing asks which of oids the org's repo lacks, in calls of at most
// perCall.
func listMissing(ctx context.Context, c *remoteapi.Client, oids []string, perCall int) ([]string, error) {
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
	client   *remoteapi.Client
	dir      string
	byOID    map[string]string // blob id → a file holding it
	sizes    map[string]int    // file → its size
	limits   remoteapi.Limits
	progress io.Writer
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
	fmt.Fprintf(u.progress, "Uploading %d files (%s)…\n", len(oids), byteSize(total))
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
		if remoteapi.BlobID(content) != oid {
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

// selectFiles is the files a publish sends, slash-separated, relative to dir,
// sorted: the repo's own, as remoteapi.Publishable says, the same on every
// machine, with or without git. leftOut counts the folder's other files, which
// a publish does not carry. A symlink, or anything but a regular file, among
// the repo's own is refused by name: never skipped, which would delete it on the
// server, nor followed out of the repo.
func selectFiles(dir string) (files []string, leftOut int, err error) {
	var refused []string
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch {
		case rel == ".":
			return nil
		case d.IsDir() && (strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules"):
			return filepath.SkipDir // nothing under it is publishable: not worth walking
		case d.IsDir():
			return nil
		case !remoteapi.Publishable(rel):
			leftOut++
		case d.Type()&fs.ModeSymlink != 0:
			refused = append(refused, rel+" (a symlink)")
		case !d.Type().IsRegular():
			refused = append(refused, rel+" (not a regular file)")
		default:
			files = append(files, rel)
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	if len(refused) > 0 {
		return nil, 0, fmt.Errorf("a publish carries regular files only; replace these with the files themselves: %s", strings.Join(refused, ", "))
	}
	slices.Sort(files)
	return files, leftOut, nil
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

func (s publishedState) last(r remoteapi.Remote) string { return s.read()[r.String()] }

// record notes revision as published from here. Best-effort: without it, the
// next publish asks before replacing what it cannot tell was its own.
func (s publishedState) record(r remoteapi.Remote, revision string) {
	m := s.read()
	m[r.String()] = revision
	raw, err := jsonkit.Marshal(m)
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(s.path), 0o755) == nil { //nolint:gosec // G301: anfra's own state folder
		_ = os.WriteFile(s.path, raw, 0o600)
	}
}

func who(p *remoteapi.Person) string {
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
