// Package repoenv reads a repo's root .env: each person's settings for the
// repo, below the environment (docs/designs/telemetry.md, setting storages).
//
// Only anfra's names are read (loaded): telemetry's OTEL_* and ANFRA_EXPERIMENTAL_TRACE_*, and
// anfra's other switches. The file may be an app's too, and its other names are
// that app's: read, an app's PGPASSWORD would reach canal-query, which inherits
// anfra's environment, and PATH would choose the git anfra runs. The sidecar
// prefixes (ANFRA_NODE_ENV_*, ANFRA_CANAL_QUERY_ENV_*, ANFRA_CANAL_LAKE_ENV_*)
// aren't read either: they
// set any name in a sidecar, NODE_OPTIONS included, so whoever writes the file
// could run code there. Load sets each name read that the environment doesn't
// already set, once, when anfra starts and before anything reads a setting, so
// it applies as if it were in the environment: to anfra, to the OpenTelemetry
// SDK, and to the sidecars, which inherit anfra's environment
// (internal/sidecar).
//
// The file is git-ignored, so it is each person's. A file Git tracks is never
// read: otherwise cloning a repo would let its author choose where its traces,
// and their payloads, go. Nor is one in a Git work tree when Git can't say.
package repoenv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// FileName is the file, at the repo's root, beside .anfra/.
const FileName = ".env"

// MaxSize is the most of the file Load reads. The file may be an app's too,
// so it can be long, but anything bigger is a mistake, refused rather than read
// on every command.
const MaxSize = 1 << 20

// Loaded is what Load found.
type Loaded struct {
	// Path is the file, when there is one in a repo; "" otherwise.
	Path string
	// Set are the names Load set, in the file's order: those it reads that the
	// environment didn't set already.
	Set []string
	// Tracked is whether Git tracks the file, so Load read none of it.
	Tracked bool
	// Unchecked is whether the repo is in a Git work tree but Git couldn't say
	// whether it tracks the file (no git, say, as in anfra's Docker image), so
	// Load read none of it either.
	Unchecked bool
	// Ignored are the ANFRA_* and OTEL_* names in a file Load didn't read,
	// tracked or unchecked: what anfra warns about. Other tools' names are
	// theirs.
	Ignored []string
}

// fromFile is the entry that set each name Load set: for an error or a status
// to say where a setting came from, while the name still has its value.
var fromFile = map[string]Entry{}

// Load reads dir's .env when dir is a repo (it has .anfra/) and sets each name
// in it that the environment doesn't set. Nothing for a folder that isn't a
// repo, or a repo without the file. A line of anfra's it can't read is an
// error naming the file and line, and nothing is set. A line it can't read
// that isn't anfra's is another tool's, written in that tool's dialect, and
// is skipped.
//
// Git is asked whether it tracks the file only when the file holds an ANFRA_*
// or OTEL_* name, so a file that is only an app's costs no git process.
func Load(dir string) (Loaded, error) {
	if info, err := os.Stat(filepath.Join(dir, ".anfra")); err != nil || !info.IsDir() {
		return Loaded{}, nil
	}
	path := filepath.Join(dir, FileName)
	raw, err := read(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Loaded{}, nil
	}
	l := Loaded{Path: path}
	if err != nil {
		return l, err
	}
	entries, bad := Parse(raw)
	var anfras []string
	note := func(name string) {
		if isAnfraName(name) && !slices.Contains(anfras, name) {
			anfras = append(anfras, name)
		}
	}
	for _, e := range entries {
		note(e.Name)
	}
	for _, b := range bad {
		note(b.Name)
	}
	if len(anfras) == 0 {
		return l, nil
	}
	if l.Tracked, l.Unchecked = tracked(dir); l.Tracked || l.Unchecked {
		l.Ignored = anfras
		return l, nil
	}
	for _, b := range bad {
		if loaded(b.Name) {
			return l, fmt.Errorf("%s: %w", FileName, b)
		}
	}
	mine := map[string]bool{}
	for _, e := range entries {
		if !loaded(e.Name) {
			continue // another tool's, or a sidecar's
		}
		if _, set := os.LookupEnv(e.Name); set && !mine[e.Name] {
			continue // the environment wins
		}
		if err := os.Setenv(e.Name, e.Value); err != nil {
			return l, fmt.Errorf("%s line %d: set %s: %w", FileName, e.Line, e.Name, err)
		}
		if !mine[e.Name] {
			l.Set = append(l.Set, e.Name)
		}
		mine[e.Name] = true
		fromFile[e.Name] = e // a name given twice: the last line wins, as in dotenv
	}
	return l, nil
}

// Environ is the environment without the names Load set from the file, for
// another anfra started in the same repo: it reads the file itself, so it
// knows which of its settings came from there (Origin), and the environment
// still wins over the file for it as for this one.
func Environ() []string {
	env := os.Environ()
	return slices.DeleteFunc(env, func(kv string) bool {
		name, _, _ := strings.Cut(kv, "=")
		_, inFile := Origin(name)
		return inFile
	})
}

// isAnfraName reports whether name is one of anfra's: ANFRA_* or OTEL_*.
func isAnfraName(name string) bool {
	return strings.HasPrefix(name, "ANFRA_") || strings.HasPrefix(name, "OTEL_")
}

// sidecarPrefixes set any name in a sidecar (internal/sidecar's childEnv):
// anfranode.EnvPrefix, canalquery.EnvPrefix and canallake.EnvPrefix, which
// this package can't import.
var sidecarPrefixes = []string{"ANFRA_NODE_ENV_", "ANFRA_CANAL_QUERY_ENV_", "ANFRA_CANAL_LAKE_ENV_"}

// loaded reports whether Load reads name from the file: one of anfra's, ANFRA_*
// or OTEL_*, but not a sidecar's prefixed name.
func loaded(name string) bool {
	if !isAnfraName(name) {
		return false
	}
	for _, p := range sidecarPrefixes {
		if strings.HasPrefix(name, p) {
			return false
		}
	}
	return true
}

// Origin is where the file set name, "line 3", and whether it did: false when
// name came from the environment, or isn't set.
func Origin(name string) (string, bool) {
	e, ok := fromFile[name]
	if !ok || os.Getenv(name) != e.Value {
		return "", false
	}
	return fmt.Sprintf("line %d", e.Line), true
}

// Describe is name as something a person can unset: "NAME in .env" when the
// file set it, NAME otherwise.
func Describe(name string) string {
	if _, ok := Origin(name); ok {
		return name + " in " + FileName
	}
	return name
}

// Entry is one NAME=value line of the file.
type Entry struct {
	Name, Value string
	Line        int
}

// BadLine is a line of the file Parse couldn't read.
type BadLine struct {
	Line int
	// Name is the name the line starts with, when it starts with one: "" for a
	// line that names nothing, such as a line of another tool's dialect.
	Name   string
	Reason string
}

func (b BadLine) Error() string {
	if b.Name == "" {
		return fmt.Sprintf("line %d %s", b.Line, b.Reason)
	}
	return fmt.Sprintf("line %d, %s: %s", b.Line, b.Name, b.Reason)
}

var namePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Parse reads dotenv's format: NAME=value lines, # comments and blank lines,
// an optional "export " before the name, and a value optionally in single or
// double quotes, which may span lines. Inside double quotes, \" is a quote
// and \\ a backslash. Nothing is expanded. An unquoted value ends before " #".
// Each line that isn't one of these is a BadLine, and Parse reads on.
func Parse(raw []byte) ([]Entry, []BadLine) {
	var entries []Entry
	var bad []BadLine
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		n := i + 1
		line := strings.TrimSpace(lines[i])
		if i == 0 {
			line = strings.TrimPrefix(line, "\uFEFF")
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		name, value, hasEq := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !hasEq {
			name, _, _ = strings.Cut(line, " ")
		}
		if !namePattern.MatchString(name) {
			bad = append(bad, BadLine{Line: n, Reason: "isn't NAME=value"})
			continue
		}
		if !hasEq {
			bad = append(bad, BadLine{Line: n, Name: name, Reason: "isn't NAME=value"})
			continue
		}
		value = strings.TrimSpace(value)
		// A quoted value that doesn't close on its line goes on to the line
		// that closes it. One that never closes is this line's mistake alone,
		// and the next line is read as a line of its own.
		last := i
		if value != "" && (value[0] == '"' || value[0] == '\'') {
			buf := []byte(value)
			end, from := closeQuote(buf, 1)
			for j := i + 1; end < 0 && j < len(lines); j++ {
				buf = append(append(buf, '\n'), lines[j]...)
				if end, from = closeQuote(buf, from); end >= 0 {
					value, last = string(buf), j
				}
			}
		}
		v, err := unquote(value)
		if err != nil {
			bad = append(bad, BadLine{Line: n, Name: name, Reason: err.Error()})
			continue
		}
		entries = append(entries, Entry{Name: name, Value: v, Line: n})
		i = last
	}
	return entries, bad
}

// closeQuote is the index in v of the quote that closes v's opening one, or
// -1, looking from index from on; next is where to look on from once more is
// appended to v. Inside double quotes, a backslash escapes the character after
// it.
func closeQuote[T string | []byte](v T, from int) (end, next int) {
	q := v[0]
	i := from
	for ; i < len(v); i++ {
		switch {
		case q == '"' && v[i] == '\\':
			i++
		case v[i] == q:
			return i, i
		}
	}
	return -1, i
}

// unquote is a value as written: inside its quotes when it has them, otherwise
// up to a comment.
func unquote(v string) (string, error) {
	if v == "" {
		return "", nil
	}
	if q := v[0]; q == '"' || q == '\'' {
		end, _ := closeQuote(v, 1)
		if end < 0 {
			return "", fmt.Errorf("its value opens a %c quote it doesn't close", q)
		}
		if rest := strings.TrimSpace(v[end+1:]); rest != "" && !strings.HasPrefix(rest, "#") {
			return "", fmt.Errorf("its value has text after its closing quote")
		}
		inner := v[1:end]
		if q == '"' {
			inner = strings.NewReplacer(`\"`, `"`, `\\`, `\`).Replace(inner)
		}
		return inner, nil
	}
	if i := strings.Index(v, " #"); i >= 0 {
		v = v[:i]
	}
	return strings.TrimSpace(v), nil
}

// read is the file, refused when it isn't a regular file or is over MaxSize.
func read(path string) ([]byte, error) {
	f, err := os.Open(path) //nolint:gosec // G304: the repo's own .env
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if info, err := f.Stat(); err == nil && !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s isn't a regular file, so anfra didn't read it", FileName)
	}
	raw, err := io.ReadAll(io.LimitReader(f, MaxSize+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxSize {
		return nil, fmt.Errorf("%s is over %d KiB, so anfra didn't read it. A settings file is a few lines, so check that it's the file you meant", FileName, MaxSize>>10)
	}
	return raw, nil
}

// tracked reports whether Git tracks dir's .env, and unchecked whether it
// couldn't tell: dir is in a Git work tree, but there is no git, or git
// failed (an owner it distrusts, say). A folder in no work tree tracks
// nothing.
func tracked(dir string) (tracked, unchecked bool) {
	if !inWorkTree(dir) {
		return false, false
	}
	if _, err := exec.LookPath("git"); err != nil {
		return false, true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "ls-files", "--error-unmatch", "--", FileName) //nolint:gosec // G204: git, with fixed arguments, never a shell
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	err := cmd.Run()
	if err == nil {
		return true, false
	}
	if exit, ok := errors.AsType[*exec.ExitError](err); ok && exit.ExitCode() == 1 {
		return false, false // ls-files' answer for a file it doesn't track
	}
	return false, true
}

// inWorkTree reports whether dir or a folder above it has .git.
func inWorkTree(dir string) bool {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return true
	}
	for {
		if _, err := os.Lstat(filepath.Join(abs, ".git")); err == nil {
			return true
		}
		up := filepath.Dir(abs)
		if up == abs {
			return false
		}
		abs = up
	}
}
