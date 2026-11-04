package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"

	"github.com/holistics/anfra/internal/home"
	"github.com/holistics/anfra/shared/jsonkit"
)

// historyKept is how many entries a console's history keeps, as bash keeps
// 1,000 lines and psql 500.
const historyKept = 1000

// consoleHistory is anfra console's entries, kept between sessions
// (docs/designs/console.md): a file in anfra's home, one for every repo, as
// psql's and a shell's. Each entry is a JSON string on a line of its own, so
// an entry of several lines, AQL, is one record; the newest last. Every
// session appends as each entry runs, so a session killed keeps what it ran,
// and two at once both keep theirs.
type consoleHistory struct{ path string }

func openConsoleHistory() consoleHistory {
	return consoleHistory{path: filepath.Join(home.Dir(), "console_history")}
}

// load is the history's last historyKept entries, oldest first: none when the
// file is missing or can't be read, a line that isn't an entry skipped. A file
// grown past twice the limit is cut back to it, so it never grows unbounded
// though sessions only append.
func (h consoleHistory) load() []string {
	b, err := os.ReadFile(h.path)
	if err != nil {
		return nil
	}
	var entries []string
	lines := 0
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 64*1024), 16<<20)
	for sc.Scan() {
		lines++
		var e string
		if jsonkit.Unmarshal(sc.Bytes(), &e) == nil && e != "" {
			entries = append(entries, e)
		}
	}
	if len(entries) > historyKept {
		entries = entries[len(entries)-historyKept:]
	}
	if lines > 2*historyKept {
		h.rewrite(entries)
	}
	return entries
}

// add appends entry to the history, best-effort: a history that can't be
// written leaves the session as it is.
func (h consoleHistory) add(entry string) {
	line, err := jsonkit.Marshal(entry)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(h.path), 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(h.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600) // entries may hold what's private
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

// rewrite replaces the history with entries, through a file beside it renamed
// over it, so a reader never sees half of one.
func (h consoleHistory) rewrite(entries []string) {
	var b bytes.Buffer
	for _, e := range entries {
		line, err := jsonkit.Marshal(e)
		if err != nil {
			return
		}
		b.Write(append(line, '\n'))
	}
	tmp, err := os.CreateTemp(filepath.Dir(h.path), ".console_history-*")
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b.Bytes()); err != nil {
		_ = tmp.Close()
		return
	}
	if err := tmp.Close(); err != nil {
		return
	}
	_ = os.Rename(tmp.Name(), h.path)
}
