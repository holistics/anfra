// Package motherduck is anfra's use of a person's MotherDuck account, to copy a
// DuckDB file there for an org to query: MotherDuck's own CLI signs them in and
// uploads, and MotherDuck's REST API makes the org a token of its own
// (docs/designs/motherduck-copy.md).
//
// It never changes what the account already has. Every statement it sends is in
// this file, and none replaces, drops, alters or deletes: a database is created
// only under a name that is free (CREATE DATABASE, which fails on one that is
// not), and a local file is only read (attached READ_ONLY). The one thing it
// removes is a token it created itself, which nothing uses yet (API.RevokeToken).
package motherduck

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/holistics/anfra/shared/jsonkit"
)

// CLI is MotherDuck's command-line tool, `motherduck`, at Path.
type CLI struct {
	Path string
	// Stdin, Stdout and Stderr are the terminal Login hands to the CLI.
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	// run runs the CLI with args and answers its standard output; nil runs Path.
	// Tests replace it.
	run func(ctx context.Context, args ...string) ([]byte, error)
}

// Find looks for the CLI: on PATH, else where MotherDuck's installer puts it,
// ${MOTHERDUCK_HOME:-~/.motherduck}/bin, which a shell started before the
// install does not have on its PATH.
func Find() (string, bool) {
	name := "motherduck"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, true
	}
	home := os.Getenv("MOTHERDUCK_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		home = filepath.Join(userHome, ".motherduck")
	}
	path := filepath.Join(home, "bin", name)
	info, err := os.Stat(path) //nolint:gosec // G703: where MotherDuck's installer puts its CLI
	if err == nil && info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode()&0o111 != 0) {
		return path, true
	}
	return "", false
}

// InstallCommand is MotherDuck's command that installs the CLI, without the
// DuckDB CLI it otherwise installs too, which anfra does not need.
func InstallCommand() string {
	if runtime.GOOS == "windows" {
		return `powershell -c "$env:SKIP_DUCKDB_CLI=1; irm https://install.motherduck.com | iex"`
	}
	return "curl -s https://install.motherduck.com | SKIP_DUCKDB_CLI=1 sh"
}

// Version is the CLI's version, as it says it.
func (c CLI) Version(ctx context.Context) (string, error) {
	out, err := c.exec(ctx, "--version")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Status is who the CLI is signed in as.
type Status struct {
	Authenticated bool   `json:"authenticated"`
	User          string `json:"user"`
	Organization  string `json:"organization_name"`
}

// Status asks the CLI who it is signed in as. Signed out is a Status, not an
// error.
func (c CLI) Status(ctx context.Context) (Status, error) {
	out, err := c.exec(ctx, "status", "-o", "json")
	if err != nil {
		return Status{}, err
	}
	var s Status
	if err := jsonkit.Unmarshal(bytes.TrimSpace(out), &s); err != nil {
		return Status{}, fmt.Errorf("read the MotherDuck CLI's status: %w", err)
	}
	return s, nil
}

// Login hands the terminal to `motherduck login`: MotherDuck's own sign-in, in
// the browser, which also signs up.
func (c CLI) Login(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, c.Path, "login") //nolint:gosec // G204: MotherDuck's CLI, found by Find
	cmd.Stdin, cmd.Stdout, cmd.Stderr = c.Stdin, c.Stdout, c.Stderr
	return cmd.Run()
}

// Session is what an upload needs of the signed-in user's account, in one call,
// since each call connects to MotherDuck anew, which takes seconds: their token,
// for the REST API, a short-lived one which reads and writes everything of
// theirs; and the names of their databases. The token is never shown, and no
// error here carries the CLI's output, which would hold it.
func (c CLI) Session(ctx context.Context) (token string, databases []string, err error) {
	out, err := c.exec(ctx, "query", "-o", "json",
		"PRAGMA PRINT_MD_TOKEN; SELECT list(name ORDER BY name) AS names FROM md_information_schema.databases")
	if err != nil {
		var failed QueryError
		if jsonkit.Unmarshal(bytes.TrimSpace(out), &failed) == nil && failed.Message != "" && !strings.Contains(failed.Message, "motherduck_token") {
			return "", nil, &failed
		}
		return "", nil, errors.New("the MotherDuck CLI did not give its token")
	}
	// The pragma prints a line of shell, then an empty JSON array; the
	// SELECT, one JSON row.
	var last string
	for line := range strings.SplitSeq(string(out), "\n") {
		line = strings.TrimSpace(line)
		if t, ok := strings.CutPrefix(line, "export motherduck_token="); ok && t != "" && !strings.ContainsAny(t, " \t'\"") {
			token = t
		} else if strings.HasPrefix(line, "[") {
			last = line
		}
	}
	if token == "" {
		return "", nil, errors.New("the MotherDuck CLI did not give its token")
	}
	var rows []struct {
		Names []string `json:"names"`
	}
	if err := jsonkit.Unmarshal([]byte(last), &rows); err != nil || len(rows) != 1 {
		return "", nil, errors.New("the MotherDuck CLI did not list the account's databases")
	}
	return token, rows[0].Names, nil
}

// DatabaseExists reports whether the account has a database named name, in any
// case: DuckDB's names are not case-sensitive.
func (c CLI) DatabaseExists(ctx context.Context, name string) (bool, error) {
	if !validName(name) {
		return false, fmt.Errorf("%q is not a database name anfra makes", name)
	}
	rows, err := c.query(ctx, "SELECT count(*) AS n FROM md_information_schema.databases WHERE lower(name) = '"+name+"'")
	if err != nil {
		return false, err
	}
	if len(rows) != 1 {
		return false, errors.New("the MotherDuck CLI answered no count of databases")
	}
	n, err := strconv.Atoi(fmt.Sprint(rows[0]["n"]))
	if err != nil {
		return false, fmt.Errorf("the MotherDuck CLI answered a count of databases that is not one: %v", rows[0]["n"])
	}
	return n > 0, nil
}

// CreateDatabaseFrom uploads the DuckDB file at path to the account, as a new
// database named name. The file is attached READ_ONLY, so it is never written;
// CREATE DATABASE, without OR REPLACE, fails when name is taken, whatever
// checked it before.
func (c CLI) CreateDatabaseFrom(ctx context.Context, name, path string) error {
	if !validName(name) {
		return fmt.Errorf("%q is not a database name anfra makes", name)
	}
	if !filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\n\r") {
		return fmt.Errorf("%q is not a file anfra uploads", path)
	}
	alias, err := uploadAlias()
	if err != nil {
		return err
	}
	_, err = c.query(ctx, "ATTACH "+quote(path)+" AS "+alias+" (READ_ONLY); CREATE DATABASE "+name+" FROM "+alias)
	return err
}

// namePattern is the database names anfra makes (DatabaseName): ones SQL takes
// unquoted, so a name never needs escaping.
var namePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

func validName(name string) bool { return namePattern.MatchString(name) }

// DatabaseName is a database name made from s, a file's name: lowercase
// letters, digits and underscores, starting with no digit.
func DatabaseName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	name := strings.Trim(b.String(), "_")
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		name = "db_" + name
	}
	return strings.TrimRight(name[:min(len(name), 56)], "_") // room for a suffix: _2…_99
}

// uploadAlias is a name for the local file while it is attached, unlike any of
// the account's.
func uploadAlias() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "anfra_upload_" + hex.EncodeToString(b), nil
}

// quote is s as an SQL string literal.
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// queryTimeout is the longest the CLI lets one query run, which an upload of a
// large file can take; interrupting the publish stops it sooner.
const queryTimeout = "86400"

// query runs sql through the CLI and answers the last statement's rows.
func (c CLI) query(ctx context.Context, sql string) ([]map[string]any, error) {
	out, err := c.exec(ctx, "query", "--timeout", queryTimeout, "-o", "json", sql)
	if err != nil {
		var failed QueryError
		if jsonkit.Unmarshal(bytes.TrimSpace(out), &failed) == nil && failed.Message != "" {
			return nil, &failed
		}
		return nil, err
	}
	// One line of JSON per statement that answers rows; none for one that
	// answers nothing.
	var rows []map[string]any
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		rows = nil
		if err := jsonkit.Unmarshal([]byte(line), &rows); err != nil {
			return nil, fmt.Errorf("read the MotherDuck CLI's answer: %w", err)
		}
	}
	return rows, nil
}

// QueryError is a statement MotherDuck or DuckDB refused, with its message.
type QueryError struct {
	Message string `json:"error"`
}

func (e *QueryError) Error() string { return e.Message }

var lockPattern = regexp.MustCompile(`Conflicting lock is held in (.+?) \(PID (\d+)\)`)

// LockedBy reports whether err is a local file being held by another process,
// which DuckDB then does not open at all, and names that process.
func LockedBy(err error) (holder string, pid int, ok bool) {
	var q *QueryError
	if !errors.As(err, &q) || !strings.Contains(q.Message, "Could not set lock on file") {
		return "", 0, false
	}
	m := lockPattern.FindStringSubmatch(q.Message)
	if m == nil {
		return "", 0, true
	}
	pid, _ = strconv.Atoi(m[2])
	return m[1], pid, true
}

func (c CLI) exec(ctx context.Context, args ...string) ([]byte, error) {
	if c.run != nil {
		return c.run(ctx, args...)
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, c.Path, args...) //nolint:gosec // G204: MotherDuck's CLI, found by Find; SQL is built here only
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return stdout.Bytes(), ctx.Err()
		}
		// The CLI says a failed query on stdout, as JSON, and anything else on
		// stderr.
		if msg := strings.TrimSpace(stderr.String()); msg != "" && stdout.Len() == 0 {
			return stdout.Bytes(), fmt.Errorf("motherduck %s: %s", args[0], msg)
		}
		return stdout.Bytes(), fmt.Errorf("motherduck %s: %w", args[0], err)
	}
	return stdout.Bytes(), nil
}
