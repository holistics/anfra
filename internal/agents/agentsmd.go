package agents

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The anfra block of a repo's AGENTS.md, between its markers. anfra replaces
// only this block, so the rest of the file stays the repo's own.
const (
	blockBegin = "<!-- anfra:begin -->"
	blockEnd   = "<!-- anfra:end -->"

	block = blockBegin + `
## anfra

This is an anfra repo.

- Use the anfra skills for work in this repo. They know anfra's languages and commands, so you'll work more
  accurately and efficiently with them than from memory.
- Docs: anfra at https://anfra.ai/docs, AML and AQL at https://amql.org/docs.

Keeping the skills working:

- Once per session, before your first anfra command, run ` + "`anfra version --skills <version>`" + `, with the version
  from the ` + "`anfra skills`" + ` line at the end of an anfra skill you've loaded, or ` + "`--skills unknown`" + ` if it has no such
  line. Follow the hints it prints. If anfra doesn't know ` + "`--skills`" + `, it's out of date: ask the user to run
  ` + "`anfra update`" + `.
- If the anfra skills aren't available to you, run ` + "`anfra agent setup --agent <your id>`" + ` and follow what it says,
  then ask the user to restart you. If that fails, install them as ` + readmeURL + ` says
  for the agent you are.
` + blockEnd + "\n"

	// claudeImport makes Claude Code read AGENTS.md, which it does on its own
	// only when no CLAUDE.md exists.
	claudeImport = "@AGENTS.md"
)

// Block is the state of a repo's anfra block in AGENTS.md, and of the import
// in its CLAUDE.md.
type Block struct {
	State BlockState
	// NoImport: the repo has a CLAUDE.md, which doesn't import AGENTS.md.
	NoImport bool
}

// BlockState is the state of the anfra block itself.
type BlockState string

const (
	BlockMissing  BlockState = "missing"
	BlockOutdated BlockState = "outdated" // other than what this anfra writes
	BlockUpToDate BlockState = "up-to-date"
)

// CheckBlock reports the anfra block in repoDir's AGENTS.md, and the import in
// its CLAUDE.md, when it has one.
func CheckBlock(repoDir string) (Block, error) {
	md, err := readRegular(filepath.Join(repoDir, "AGENTS.md"))
	if err != nil {
		return Block{}, err
	}
	var b Block
	switch current, found := currentBlock(md); {
	case !found:
		b.State = BlockMissing
	case current != block:
		b.State = BlockOutdated
	default:
		b.State = BlockUpToDate
	}
	claude, err := readRegular(filepath.Join(repoDir, "CLAUDE.md"))
	if err != nil {
		return Block{}, err
	}
	b.NoImport = claude != nil && !hasLine(claude, claudeImport)
	return b, nil
}

// WriteBlock writes the anfra block into repoDir's AGENTS.md: it adds it to the
// file, creating it, or replaces the block that is there. It adds the import
// to the repo's CLAUDE.md, when it has one. It refuses a symlink, which would
// write outside the repo. It says whether it wrote the block, and the import.
func WriteBlock(repoDir string) (wroteBlock, wroteImport bool, err error) {
	path := filepath.Join(repoDir, "AGENTS.md")
	md, err := readRegular(path)
	if err != nil {
		return false, false, err
	}
	if next := withBlock(md); !bytes.Equal(next, md) {
		if err := os.WriteFile(path, next, 0o644); err != nil { //nolint:gosec // G306: a committed doc
			return false, false, err
		}
		wroteBlock = true
	}

	claudePath := filepath.Join(repoDir, "CLAUDE.md")
	claude, err := readRegular(claudePath)
	if err != nil || claude == nil || hasLine(claude, claudeImport) {
		return wroteBlock, false, err
	}
	sep := "\n"
	if len(claude) > 0 && !bytes.HasSuffix(claude, []byte("\n")) {
		sep = "\n\n"
	}
	if err := os.WriteFile(claudePath, append(claude, []byte(sep+claudeImport+"\n")...), 0o644); err != nil { //nolint:gosec // G306: as above
		return wroteBlock, false, err
	}
	return wroteBlock, true, nil
}

// readRegular reads a file of the repo: nil when it doesn't exist, an error
// when it is a symlink or not a regular file.
func readRegular(path string) ([]byte, error) {
	fi, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file (a symlink?): anfra writes only the repo's own files", filepath.Base(path))
	}
	return os.ReadFile(path) //nolint:gosec // G304: a file of the repo being set up
}

// currentBlock is the anfra block in md, markers and the line end after them
// included; false when it has none.
func currentBlock(md []byte) (string, bool) {
	s := string(md)
	begin := strings.Index(s, blockBegin)
	if begin < 0 {
		return "", false
	}
	end := strings.Index(s[begin:], blockEnd)
	if end < 0 {
		return "", false
	}
	end += begin + len(blockEnd)
	if strings.HasPrefix(s[end:], "\n") {
		end++
	}
	return s[begin:end], true
}

// withBlock is md with this anfra's block: in place of the block it has, else
// appended after a blank line.
func withBlock(md []byte) []byte {
	s := string(md)
	if current, ok := currentBlock(md); ok {
		return []byte(strings.Replace(s, current, block, 1))
	}
	switch {
	case s == "", strings.HasSuffix(s, "\n\n"):
	case strings.HasSuffix(s, "\n"):
		s += "\n"
	default:
		s += "\n\n"
	}
	return []byte(s + block)
}

func hasLine(md []byte, line string) bool {
	for l := range strings.SplitSeq(string(md), "\n") {
		if strings.TrimSpace(l) == line {
			return true
		}
	}
	return false
}
