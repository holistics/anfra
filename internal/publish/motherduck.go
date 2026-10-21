package publish

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/motherduck"
)

// MotherDuck is a person's MotherDuck account, as copying a DuckDB file there
// needs it (motherduck.Account). Nil offers no copy: without a terminal, nobody
// is there to sign in.
type MotherDuck interface {
	Find(ctx context.Context) (version string, ok bool)
	Status(ctx context.Context) (motherduck.Status, error)
	Login(ctx context.Context) error
	Session(ctx context.Context) (token string, databases []string, err error)
	DatabaseExists(ctx context.Context, name string) (bool, error)
	CreateDatabaseFrom(ctx context.Context, name, path string) error
	CreateReadToken(ctx context.Context, session, username, name, description string) (motherduck.Token, error)
	RevokeToken(ctx context.Context, session, username, id string) error
}

// duckdbFile is a DuckDB file on this machine, and the data sources that read it.
type duckdbFile struct {
	path    string // absolute
	shown   string // as data_sources.yml has it
	info    os.FileInfo
	sources []datasource.DataSource
}

// duckdbFiles groups DuckDB data sources by the file they read, one file under
// several paths (a symlink, a hard link) being one. rest is those that read no
// file here: in memory, already on MotherDuck, or a file not found.
func duckdbFiles(dir string, sources []datasource.DataSource) (files []duckdbFile, rest []datasource.DataSource) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, sources
	}
next:
	for _, d := range sources {
		configured, _ := d.Connection["path"].(string)
		path, _, _ := strings.Cut(configured, "?")
		shown := path
		lower := strings.ToLower(path)
		if path == "" || path == ":memory:" || strings.HasPrefix(lower, "md:") || strings.HasPrefix(lower, "motherduck:") {
			rest = append(rest, d)
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(abs, path) // canal-query opens it from anfra's folder, the repo's
		}
		path, err = filepath.EvalSymlinks(path) // the file itself, named as it is
		if err != nil {
			rest = append(rest, d)
			continue
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			rest = append(rest, d)
			continue
		}
		for i := range files {
			if os.SameFile(files[i].info, info) {
				files[i].sources = append(files[i].sources, d)
				continue next
			}
		}
		files = append(files, duckdbFile{path: path, shown: shown, info: info, sources: []datasource.DataSource{d}})
	}
	return files, rest
}

// mdCopy copies DuckDB files to the person's MotherDuck account, and gives the
// org data sources on the copies, for one publish (docs/designs/motherduck-copy.md).
//
// It changes nothing the account had: each copy is a new database under a name
// that is free, the local file is only read, and the one thing it removes is
// the token it made, when no data source came to use it (finish). What fails on
// MotherDuck's side is said, and the publish goes on without that data source.
type mdCopy struct {
	md    MotherDuck
	c     *cloudapi.Client
	org   cloudapi.Org
	p     *cliui.Printer
	ask   Prompt
	input Input // nil: nobody to type a token or a name

	ready, gaveUp bool // signed in; or failed to, and not to be tried again
	// seen is the account the CLI was signed in to when first looked at, which
	// the first question names; status, once signed in, the one uploaded to; and
	// fresh, that the person signed in to it in this publish, choosing it.
	// looking is that first look, started ahead (lookAhead): done when closed.
	looking chan struct{}
	seen    *motherduck.Status
	status  motherduck.Status
	fresh   bool
	session string // the person's own token, for the REST API only
	// databases is the account's, by name in lower case, as listed in signing in
	// and added to by each upload: one listing for the publish, since every call
	// connects to MotherDuck anew, which takes seconds.
	databases map[string]bool
	// all is yes to every DuckDB data source still to come, to the account
	// named when it was chosen.
	all       bool
	named     string // the account the last yes was to
	token     *motherduck.Token
	tokenNew  bool // token was made by this publish, not pasted
	tokenUsed bool // a data source on the org holds token
}

// offer offers to copy f to MotherDuck, and answers the data sources it then
// created on the org. An error is the context's, or the org's (forbidden).
func (m *mdCopy) offer(ctx context.Context, f duckdbFile, rest int) (created []string, err error) {
	p := m.p
	names := make([]string, len(f.sources))
	for i, d := range f.sources {
		names[i] = d.Name
	}
	if len(names) == 1 {
		p.Warn("Data source %s (duckdb) uses a local file that the org's server can't read.", names[0])
	} else {
		p.Warn("Data sources %s (duckdb) use %s, a local file that the org's server can't read.", strings.Join(names, ", "), f.shown)
	}
	instead := func() {
		p.Note("To use it on the org, load its data into a warehouse the server can reach, and add that to the org as %s.", strings.Join(names, ", "))
	}
	if m.md == nil {
		instead()
		p.Note("Or run anfra publish in a terminal to upload it to MotherDuck.")
		return nil, nil
	}
	if m.gaveUp {
		instead()
		return nil, nil
	}
	acct := m.account(ctx)
	if yes, err := m.askUpload(ctx, f, names, acct, rest); !yes {
		if err == nil {
			instead()
		}
		return nil, err
	}
	if ok, err := m.signIn(ctx); !ok {
		if err == nil {
			instead()
		}
		return nil, err
	}
	if !m.fresh && (acct == nil || acct.User != m.status.User) {
		// Agreed to without this account named, as when the CLI was installed
		// since: asked again, naming it.
		if yes, err := m.askUpload(ctx, f, names, &m.status, rest); !yes {
			if err == nil {
				instead()
			}
			return nil, err
		}
	}
	// The org's token first: settled, or not, before anything is uploaded.
	if ok, err := m.orgToken(ctx, names); !ok {
		return nil, err
	}
	name, ok, err := m.upload(ctx, f)
	if !ok {
		return nil, err
	}
	for _, d := range f.sources {
		copied := datasource.DataSource{Name: d.Name, DBType: "motherduck", Connection: map[string]any{"token": m.token.Secret, "dbname": name}}
		done, err := createDataSource(ctx, m.c, copied, p, m.ask)
		if err != nil {
			return created, err
		}
		if done {
			created = append(created, d.Name)
			m.tokenUsed = true
		}
	}
	if len(created) > 0 {
		p.Note("The org will query this snapshot. Later changes to the local file are not uploaded.")
	}
	return created, nil
}

// errSwitchAccount ends the publish for the person to switch the MotherDuck
// account the CLI is signed in to, which anfra leaves to them: signing the CLI
// out signs them out wherever they use it.
var errSwitchAccount = errors.New("not published: to upload to another MotherDuck account, run motherduck logout, then motherduck login, then publish again")

// askUpload asks whether to upload f, naming the account it goes to when one is
// known, with the choice then to quit to switch it, and, with rest files still
// to come, to say yes to them all. yes false and no error is no; an error,
// quitting or interrupted. After yes to all, it is not asked again, but for an
// account no question named yet.
func (m *mdCopy) askUpload(ctx context.Context, f duckdbFile, names []string, acct *motherduck.Status, rest int) (yes bool, err error) {
	if m.all && (acct == nil || m.named == acct.User) {
		return true, nil
	}
	where := "MotherDuck"
	if acct != nil {
		where = "your MotherDuck account " + account(*acct)
	}
	no := "no, skip uploading this data source"
	if len(names) > 1 {
		no = "no, skip uploading these data sources"
	}
	const all = "all: yes to this and all remaining duckdb data sources"
	choices := []string{"yes"}
	if rest > 0 {
		choices = append(choices, all)
	}
	if acct != nil {
		choices = append(choices, "quit: switch accounts with motherduck logout and motherduck login first, then publish again")
	}
	choices = append(choices, no)
	choice, asked := m.ask(fmt.Sprintf("Upload %s (%s) to %s, and give the anfra org read-only access to it?",
		f.shown, byteSize(int(f.info.Size())), where), choices...)
	switch {
	case ctx.Err() != nil:
		return false, ctx.Err()
	case !asked:
		return false, nil
	case strings.HasPrefix(choices[choice], "quit"):
		return false, errSwitchAccount
	case choices[choice] == no:
		m.all = false // no to an account named only now: to the rest neither
		return false, nil
	}
	if acct != nil {
		m.named = acct.User
	}
	m.all = m.all || choices[choice] == all
	return true, nil
}

// account is the MotherDuck account uploads go to, for the question to name:
// the one signed in to, or before that, the one the CLI was signed in to when
// first looked at. nil with no CLI, or none signed in. Looking only asks the
// CLI, and sends nothing of the repo's anywhere.
func (m *mdCopy) account(ctx context.Context) *motherduck.Status {
	if m.ready {
		return &m.status
	}
	m.lookAhead(ctx)
	if m.looking == nil {
		return nil // no MotherDuck to look at
	}
	select {
	case <-m.looking:
	case <-ctx.Done():
	}
	return m.seen
}

// lookAhead starts looking up the account the CLI is signed in to, which takes
// seconds, so it is done by when the first question needs it: started once
// DuckDB data sources are found, while other questions are on screen. Nothing
// else calls the CLI until it is done (account waits for it).
func (m *mdCopy) lookAhead(ctx context.Context) {
	if m.looking != nil || m.md == nil {
		return
	}
	m.looking = make(chan struct{})
	go func() {
		defer close(m.looking)
		if _, found := m.md.Find(ctx); found {
			if s, err := m.md.Status(ctx); err == nil && s.Authenticated && s.User != "" {
				m.seen = &s
			}
		}
	}()
}

// signIn finds the CLI and the person signed in to it, once per publish,
// helping with either. Not ok and no error, this data source is not uploaded:
// declined, which a later one may not be, or failed, which is not tried again.
func (m *mdCopy) signIn(ctx context.Context) (ok bool, err error) {
	if m.ready {
		return true, nil
	}
	if m.gaveUp {
		return false, nil
	}
	p := m.p
	giveUp := func() (bool, error) {
		m.gaveUp = true
		return false, ctx.Err()
	}
	if m.seen != nil {
		// Signed in when looked at, moments ago: not looked at again.
		return m.listSession(ctx, *m.seen)
	}
	version, found := m.md.Find(ctx)
	installed := found
	for !found {
		p.Warn("Uploading to MotherDuck needs the MotherDuck CLI, which is not installed.")
		p.Note("Install it in another terminal, then continue here:")
		p.Detail(motherduck.InstallCommand())
		if yes, _ := Confirm(m.ask, "Continue once it is installed?"); !yes {
			return false, ctx.Err()
		}
		version, found = m.md.Find(ctx)
	}
	if !installed {
		p.Success("Found the MotherDuck CLI %s.", version)
	}
	status, err := m.md.Status(ctx)
	if err != nil {
		p.Fail("Could not check who the MotherDuck CLI is signed in as: %v", err)
		return giveUp()
	}
	if !status.Authenticated {
		if yes, _ := Confirm(m.ask, "Sign in to MotherDuck? It opens your browser, where you can also sign up for free, with no credit card."); !yes {
			return false, ctx.Err()
		}
		p.Step("Running motherduck login…")
		if err := m.md.Login(ctx); err != nil && ctx.Err() == nil {
			p.Fail("motherduck login failed: %v", err)
		}
		if status, err = m.md.Status(ctx); err != nil || !status.Authenticated {
			p.Fail("Not signed in to MotherDuck, so nothing will be uploaded.")
			return giveUp()
		}
		m.fresh = true
	}
	if status.User == "" {
		p.Fail("The MotherDuck CLI did not report who is signed in, so nothing will be uploaded.")
		return giveUp()
	}
	if m.fresh {
		m.named = status.User // chosen in signing in: named as much as by a question
		p.Success("Signed in to MotherDuck as %s.", account(status))
	}
	return m.listSession(ctx, status)
}

// listSession gets, for the account signed in to, the person's token and the
// names of their databases, in one call.
func (m *mdCopy) listSession(ctx context.Context, status motherduck.Status) (ok bool, err error) {
	token, names, err := m.md.Session(ctx)
	if err != nil {
		m.p.Fail("%v, so nothing will be uploaded.", err)
		m.gaveUp = true
		return false, ctx.Err()
	}
	m.databases = make(map[string]bool, len(names))
	for _, n := range names {
		m.databases[strings.ToLower(n)] = true
	}
	m.session, m.status, m.ready = token, status, true
	return true, nil
}

// account names a MotherDuck account as the person knows it: its user, and
// its organization when there is one.
func account(s motherduck.Status) string {
	if s.Organization != "" {
		return s.User + " (" + s.Organization + ")"
	}
	return s.User
}

// upload uploads f as a new database, under the first free name from its
// file's. Not ok, nothing was uploaded, or what was is said.
func (m *mdCopy) upload(ctx context.Context, f duckdbFile) (name string, ok bool, err error) {
	p := m.p
	name, err = m.freeName(motherduck.DatabaseName(strings.TrimSuffix(filepath.Base(f.path), filepath.Ext(f.path))))
	if err != nil {
		p.Fail("Could not find a free database name in your MotherDuck account:")
		p.Detail(err.Error())
		return "", false, nil
	}
	p.Step("Uploading %s to MotherDuck as database %s… (a large file takes a while)", f.shown, name)
	err = m.md.CreateDatabaseFrom(ctx, name, f.path)
	if holder, pid, locked := motherduck.LockedBy(err); locked {
		if !strings.HasPrefix(filepath.Base(holder), "canal-query") {
			p.Fail("%s is open in another program (%s, PID %d), so it can't be read right now.", f.shown, holder, pid)
			p.Note("Close it, then publish again to upload the file.")
			return "", false, nil
		}
		// anfra's own query engine, from a running anfra serve: it only reads
		// the file, so a copy of it is the file.
		p.Note("anfra serve has the file open, so a temporary copy of it is uploaded instead.")
		copied, cleanup, cerr := copyForUpload(f.path)
		if cerr != nil {
			p.Fail("Could not copy %s: %v", f.shown, cerr)
			p.Note("Stop anfra serve, then publish again to upload the file.")
			return "", false, nil
		}
		defer cleanup()
		err = m.md.CreateDatabaseFrom(ctx, name, copied)
	}
	if err != nil {
		if ctx.Err() != nil {
			p.Warn("Upload interrupted. The database %s may be partly created in your MotherDuck account. anfra never deletes anything there, so drop it yourself if you don't need it.", name)
			return "", false, ctx.Err()
		}
		p.Fail("Could not upload %s:", f.shown)
		p.Detail(err.Error())
		if strings.Contains(err.Error(), "already exists") {
			m.databases[name] = true
			p.Note("Another database named %s was created in the meantime. It was not changed.", name) // not this upload's: never call it partial
		} else if exists, xerr := m.md.DatabaseExists(ctx, name); xerr == nil && exists {
			p.Warn("The database %s may be partly created in your MotherDuck account. anfra never deletes anything there, so drop it yourself if you don't need it.", name)
		}
		return "", false, nil
	}
	m.databases[name] = true
	p.Success("Uploaded %s to MotherDuck as database %s.", f.shown, name)
	return name, true, nil
}

// freeName is base, or base_2, base_3…, the first the account has no database
// of, as listed in signing in. One taken since fails the upload, which never
// replaces it.
func (m *mdCopy) freeName(base string) (string, error) {
	for i := 1; i < 100; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s_%d", base, i)
		}
		if !m.databases[name] {
			if i > 1 {
				m.p.Note("A database named %s already exists in your MotherDuck account, so this one will be named %s.", base, name)
			}
			return name, nil
		}
	}
	return "", fmt.Errorf("the names %s to %s_99 are all taken", base, base)
}

// orgToken settles the org's token, a read-only one, once per publish: never
// the person's own, which can write everything of theirs. anfra makes it,
// "anfra <org>". That name taken by an earlier publish's, whose secret anfra
// does not keep, the person chooses: paste a read-only token they kept, have
// anfra make one under the next free number, or under a name they type. A token
// anfra makes is shown, once, for them to keep. Not ok, nothing is uploaded.
func (m *mdCopy) orgToken(ctx context.Context, sources []string) (ok bool, err error) {
	if m.token != nil {
		return true, nil
	}
	p := m.p
	base := "anfra " + m.org.Slug
	name, next := base, 2
	for {
		t, err := m.md.CreateReadToken(ctx, m.session, m.status.User, name,
			fmt.Sprintf("Read-only access for the anfra org %s, made by anfra publish. Revoke it to cut the org off.", m.org))
		if err == nil {
			m.token, m.tokenNew = &t, true
			p.Success("Created read-only MotherDuck token %q for the org.", t.Name)
			p.Warn("Here is its secret. It is shown only once, and it can read all your MotherDuck databases, so store it " +
				"somewhere secure, like a password manager. You can paste it in a later publish:")
			p.Detail(t.Secret)
			return true, nil
		}
		if !motherduck.NameTaken(err) {
			p.Fail("Could not create a read-only MotherDuck token for the org:")
			p.Detail(err.Error())
			p.Note("Nothing was uploaded.")
			return false, ctx.Err()
		}
		p.Warn("A token named %q already exists in your MotherDuck account. anfra doesn't store token secrets, so it can't reuse it.", name)
		which, skip := "data source "+sources[0], "skip uploading this data source"
		if len(sources) > 1 {
			which, skip = "data sources "+strings.Join(sources, ", "), "skip uploading these data sources"
		}
	choose:
		for {
			nextName := fmt.Sprintf("%s %d", base, next)
			const paste, create, named = "paste a read-only token", "create new token named ", "name a new token"
			choices := []string{paste, create + strconv.Quote(nextName), named, skip}
			if m.input == nil {
				choices = []string{create + strconv.Quote(nextName), skip}
			}
			choice, asked := m.ask(fmt.Sprintf("Which token should %s use?", which), choices...)
			if err := ctx.Err(); err != nil {
				return false, err
			}
			switch {
			case !asked || choices[choice] == skip:
				p.Note("Nothing was uploaded.")
				return false, nil
			case strings.HasPrefix(choices[choice], create):
				name, next = nextName, next+1
				break choose
			case choices[choice] == named:
				typed, _ := m.input("Name for the new token:")
				if typed == "" {
					continue
				}
				name = typed
				break choose
			default: // paste
				typed, _ := m.input("Read-only MotherDuck token:")
				if typed == "" {
					continue
				}
				readOnly, err := motherduck.ReadOnly(typed)
				switch {
				case err != nil:
					p.Fail("That is %v.", err)
				case !readOnly:
					p.Fail("That token has write access. The org can only be given a read-only (Read Scaling) token.")
				default:
					m.token, m.tokenNew = &motherduck.Token{Secret: typed}, false
					p.Success("Using the pasted read-only token for the org.")
					return true, nil
				}
			}
		}
	}
}

// finish revokes the token made for the org when no data source came to use
// it, as when creating them failed: it would otherwise be left behind, reading
// all the person's databases. It runs though the publish was interrupted.
func (m *mdCopy) finish(ctx context.Context) {
	if m.token == nil || !m.tokenNew || m.tokenUsed {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	if err := m.md.RevokeToken(ctx, m.session, m.status.User, m.token.ID); err != nil {
		m.p.Warn("Could not revoke the unused MotherDuck token %q (%v). Revoke it in MotherDuck's settings.", m.token.Name, err)
		return
	}
	m.p.Note("Revoked the unused MotherDuck token %q. The secret shown above no longer works.", m.token.Name)
}

// copyForUpload copies the DuckDB file at path, and its write-ahead log if it
// has one, to a new temporary folder: for uploading while another process
// holds the file, which DuckDB then opens nowhere else. It refuses a file that
// changed while it was copied, which would be a torn copy.
func copyForUpload(path string) (copied string, cleanup func(), err error) {
	paths := []string{path, path + ".wal"}
	before := make([]os.FileInfo, len(paths))
	for i, p := range paths {
		before[i], _ = os.Stat(p) // no log: nil
	}
	dir, err := os.MkdirTemp("", "anfra-motherduck-")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { _ = os.RemoveAll(dir) }
	copied = filepath.Join(dir, filepath.Base(path))
	for i, p := range paths {
		if before[i] == nil {
			continue
		}
		if err := copyFile(p, copied+strings.TrimPrefix(p, path)); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	for i, p := range paths {
		after, _ := os.Stat(p)
		if (before[i] == nil) != (after == nil) || (after != nil && (after.Size() != before[i].Size() || !after.ModTime().Equal(before[i].ModTime()))) {
			cleanup()
			return "", nil, errors.New("it changed while it was copied")
		}
	}
	return copied, cleanup, nil
}

func copyFile(from, to string) error {
	src, err := os.Open(from)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	return dst.Close()
}
