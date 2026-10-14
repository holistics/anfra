// Package repoconfig is a repo's settings file, .anfra/config.yml: committed,
// written by hand, and by anfra only through SetString, which changes nothing
// but the value it sets.
package repoconfig

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/holistics/anfra/internal/repo"
)

// FileName is the settings file, in the repo's config folder.
const FileName = "config.yml"

// Config is what anfra reads from the settings file.
type Config struct {
	Org struct {
		// URL is the org `anfra publish` publishes the repo to, by its address on
		// its server. Never a credential.
		URL string `yaml:"url"`
	} `yaml:"org"`
}

// Path is the settings file of the repo at dir.
func Path(dir string) string { return filepath.Join(dir, repo.ConfigDirName, FileName) }

// Read is the settings of the repo at dir: none set when it has no file.
func Read(dir string) (Config, error) {
	var c Config
	raw, err := os.ReadFile(Path(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return c, fmt.Errorf("read %s: %w", Path(dir), err)
	}
	return c, nil
}

// SetString sets section.key to value in the YAML file at path, as git
// config sets a key: the file is parsed only to find where things are, and the
// new file is the old one's bytes with the value's bytes replaced, so comments,
// order and formatting are untouched; written to a lock file and renamed over the
// original, so a reader sees one or the other and two writers cannot interleave.
//
//   - No file: it is created.
//   - No section: a block for it is appended at the end, which disturbs nothing
//     above it.
//   - The key, a scalar on one line: its value's bytes are replaced.
//   - Anything else (the section without the key, flow style, a value spanning
//     lines): refused, naming the line to write by hand, rather than guessed at.
//
// value must be a plain scalar that reads back as itself; the result is parsed
// again and checked before it replaces the file.
func SetString(path, section, key, value string) error {
	original, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	edited, err := editString(original, section, key, value)
	if err != nil {
		return fmt.Errorf("%s: %w; write it by hand:\n\n%s:\n  %s: %s", path, err, section, key, value)
	}
	if err := checkString(edited, section, key, value); err != nil {
		return fmt.Errorf("%s: %w (this is a bug in anfra; the file is unchanged)", path, err)
	}
	return replaceFile(path, edited)
}

// editString is SetString's edit, on the file's bytes.
func editString(original []byte, section, key, value string) ([]byte, error) {
	block := section + ":\n  " + key + ": " + value + "\n"
	if len(bytes.TrimSpace(original)) == 0 {
		return []byte("# This repo's anfra settings: committed, and free of credentials.\n" + block), nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(original, &doc); err != nil {
		return nil, fmt.Errorf("not valid YAML: %w", err)
	}
	if len(doc.Content) == 0 { // comments only
		return appendBlock(original, block), nil
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode || root.Style&yaml.FlowStyle != 0 || root.Column != 1 {
		return nil, errors.New("its top level is not a block mapping")
	}
	sec := lookup(root, section)
	if sec == nil {
		return appendBlock(original, block), nil
	}
	if sec.Kind != yaml.MappingNode || sec.Style&yaml.FlowStyle != 0 {
		return nil, fmt.Errorf("%s is not a block mapping", section)
	}
	val := lookup(sec, key)
	if val == nil {
		return nil, fmt.Errorf("%s has no %s yet, and anfra does not insert into a section", section, key)
	}
	if val.Kind != yaml.ScalarNode || val.Tag == "!!null" || val.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		return nil, fmt.Errorf("%s.%s is not a one-line value", section, key)
	}
	start, end, err := scalarSpan(original, val)
	if err != nil {
		return nil, fmt.Errorf("%s.%s: %w", section, key, err)
	}
	return bytes.Join([][]byte{original[:start], []byte(value), original[end:]}, nil), nil
}

// lookup is the value of key in mapping m, or nil.
func lookup(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if k := m.Content[i]; k.Kind == yaml.ScalarNode && k.Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func appendBlock(original []byte, block string) []byte {
	out := bytes.Clone(original)
	if !bytes.HasSuffix(out, []byte("\n")) {
		out = append(out, '\n')
	}
	return append(out, block...)
}

// scalarSpan is where n, a one-line scalar, is in src: its bytes, quotes
// included. yaml.v3 gives a node's line and column, 1-based, the column counted
// in characters; the end is found by scanning, as the source spelling may differ
// from the value (escapes, quotes).
func scalarSpan(src []byte, n *yaml.Node) (start, end int, err error) {
	start = 0
	for line := 1; line < n.Line; line++ {
		i := bytes.IndexByte(src[start:], '\n')
		if i < 0 {
			return 0, 0, errors.New("its position is past the end of the file")
		}
		start += i + 1
	}
	for col := 1; col < n.Column; col++ {
		_, size := utf8.DecodeRune(src[start:])
		if size == 0 || src[start] == '\n' {
			return 0, 0, errors.New("its position is past the end of its line")
		}
		start += size
	}
	lineEnd := len(src)
	if i := bytes.IndexByte(src[start:], '\n'); i >= 0 {
		lineEnd = start + i
	}
	rest := src[start:lineEnd]
	switch n.Style {
	case yaml.DoubleQuotedStyle:
		for i := 1; i < len(rest); i++ {
			switch rest[i] {
			case '\\':
				i++
			case '"':
				return start, start + i + 1, nil
			}
		}
	case yaml.SingleQuotedStyle:
		for i := 1; i < len(rest); i++ {
			if rest[i] == '\'' {
				if i+1 < len(rest) && rest[i+1] == '\'' {
					i++
					continue
				}
				return start, start + i + 1, nil
			}
		}
	case 0: // plain: its spelling is its value, on one line
		if bytes.HasPrefix(rest, []byte(n.Value)) {
			return start, start + len(n.Value), nil
		}
	}
	return 0, 0, errors.New("its value does not end on its line")
}

// checkString parses edited and checks section.key reads back as value.
func checkString(edited []byte, section, key, value string) error {
	var m map[string]any
	if err := yaml.Unmarshal(edited, &m); err != nil {
		return fmt.Errorf("the edit is not valid YAML: %w", err)
	}
	sec, _ := m[section].(map[string]any)
	if got, _ := sec[key].(string); got != value {
		return fmt.Errorf("the edit reads back %s.%s as %q, not %q", section, key, got, value)
	}
	return nil
}

// replaceFile writes content to path as git writes its config: into path.lock,
// created exclusively, then renamed over path, keeping its mode.
func replaceFile(path string, content []byte) error {
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // G301: a repo's config folder, committed
		return err
	}
	lock := path + ".lock"
	f, err := os.OpenFile(lock, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("%s exists: another anfra is changing %s, or one stopped midway; remove it if none is running", lock, filepath.Base(path))
	}
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		_ = os.Remove(lock)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(lock)
		return err
	}
	if err := os.Rename(lock, path); err != nil {
		_ = os.Remove(lock)
		return err
	}
	return nil
}

// Hint is how a message names a setting, for a person to find it:
// ".anfra/config.yml org.url".
func Hint(section, key string) string {
	return repo.ConfigDirName + "/" + FileName + " " + section + "." + key
}
