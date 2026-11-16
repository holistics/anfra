package logging

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/holistics/anfra/shared/jsonkit"
)

// Entry is one entry of a repo's log: a record anfra or a sidecar wrote, or a
// line a sidecar wrote that isn't a record.
type Entry struct {
	Time    time.Time      `json:"ts,omitzero" doc:"when it was written. Absent for a sidecar's line that doesn't say."`
	Level   string         `json:"level,omitempty" doc:"the record's level: DEBUG, INFO, WARN or ERROR. Absent for a sidecar's line."`
	Message string         `json:"msg,omitempty" doc:"what happened, in a sentence; from an older anfra-node, its event's name"`
	Service string         `json:"service,omitempty" doc:"who wrote it: anfra-host (anfra itself), anfra-node, or app (a Data App's report from the browser)"`
	Event   string         `json:"event,omitempty" doc:"the event's name, such as query_cache.unavailable"`
	Attrs   map[string]any `json:"attrs,omitempty" doc:"the record's other fields, such as error and hint"`
	Line    string         `json:"line,omitempty" doc:"a line a sidecar wrote that isn't a record, as it wrote it, its time taken out"`
}

// Levels, lowest first: a filter on one keeps it and those above.
var levels = []string{"DEBUG", "INFO", "WARN", "ERROR"}

// LevelValid reports whether level names a level, in any case.
func LevelValid(level string) bool { return slices.Contains(levels, strings.ToUpper(level)) }

// Filter is which entries Read and Follow keep. A zero Filter keeps every
// one. A sidecar's line has no level and no event, so a filter on either
// leaves it out.
type Filter struct {
	Level  string    // at least this level, in any case; "" any
	Events []string  // an event named one of these, or under it (query_cache matches query_cache.swept); none any
	Since  time.Time // written at or after it; zero any
}

// Keeps reports whether f keeps e.
func (f Filter) Keeps(e Entry) bool {
	if f.Level != "" && slices.Index(levels, e.Level) < slices.Index(levels, strings.ToUpper(f.Level)) {
		return false
	}
	if len(f.Events) > 0 && !slices.ContainsFunc(f.Events, func(name string) bool {
		return e.Event == name || strings.HasPrefix(e.Event, name+".")
	}) {
		return false
	}
	return f.Since.IsZero() || !e.Time.IsZero() && !e.Time.Before(f.Since)
}

// recordFields are a record's fields an Entry holds apart from its attrs. repo
// is left out: it is the same on every entry of a repo's log.
var recordFields = map[string]bool{"ts": true, "level": true, "msg": true, "service": true, "event": true, "repo": true}

// sidecarTime is the time Go's log package starts a line with, which canal's
// services write.
const sidecarTime = "2006/01/02 15:04:05 "

// ParseEntry reads one line of the log: a record, a JSON object with its
// envelope (ts, level, msg); or another line, a sidecar's, with the time it
// starts with, if any.
func ParseEntry(line string) Entry {
	if strings.HasPrefix(line, "{") {
		var fields map[string]jsontext.Value
		if jsonkit.Unmarshal([]byte(line), &fields) == nil && fields["msg"] != nil {
			return recordEntry(fields)
		}
	}
	if len(line) > len(sidecarTime) {
		if t, err := time.ParseInLocation(sidecarTime, line[:len(sidecarTime)], time.Local); err == nil {
			return Entry{Time: t, Line: line[len(sidecarTime):]}
		}
	}
	return Entry{Line: line}
}

func recordEntry(fields map[string]jsontext.Value) Entry {
	str := func(name string) string {
		var s string
		_ = jsonkit.Unmarshal(fields[name], &s)
		return s
	}
	e := Entry{Level: strings.ToUpper(str("level")), Message: str("msg"), Service: str("service"), Event: str("event")}
	e.Time, _ = time.Parse(time.RFC3339Nano, str("ts"))
	for name, raw := range fields {
		if recordFields[name] {
			continue
		}
		var v any
		if jsonkit.Unmarshal(raw, &v) != nil {
			continue
		}
		if e.Attrs == nil {
			e.Attrs = map[string]any{}
		}
		e.Attrs[name] = v
	}
	return e
}

// readBlock is how much Read reads at a time, from the log's end back.
const readBlock = 64 << 10

// Read is the last n entries of the log at path that f keeps, oldest first
// (n 0: every one), and where its last whole line ends, which Follow carries
// on from. A last line without its newline is still being written: Follow
// hands it over once it ends. It reads from the end back, so the last
// entries of a long log come at once. No log yet is no entry.
func Read(path string, f Filter, n int) ([]Entry, int64, error) {
	file, err := os.Open(path) //nolint:gosec // G304: the repo's own log
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}

	var (
		kept     []Entry // newest first
		complete int64   // where the last whole line ends; 0 until a newline is found
		tail     []byte  // what is read and not yet split into lines
	)
	// From the end back, a block at a time, each line from its last newline.
	for end := info.Size(); end > 0; {
		start := max(end-readBlock, 0)
		block := make([]byte, end-start)
		if _, err := file.ReadAt(block, start); err != nil && !errors.Is(err, io.EOF) {
			return nil, 0, err
		}
		tail = append(block, tail...)
		end = start
		for {
			i := bytes.LastIndexByte(tail, '\n')
			if i < 0 {
				break
			}
			line := tail[i+1:]
			tail = tail[:i]
			if complete == 0 {
				complete = start + int64(i) + 1 // the line after it, if any, is still being written
				continue
			}
			keepLine(line, f, &kept)
			if n > 0 && len(kept) == n {
				slices.Reverse(kept)
				return kept, complete, nil
			}
		}
	}
	if complete > 0 { // the first line, before which there's no newline
		keepLine(tail, f, &kept)
	}
	slices.Reverse(kept)
	return kept, complete, nil
}

// keepLine adds line's entry to kept when f keeps it; a blank line has none.
func keepLine(line []byte, f Filter, kept *[]Entry) {
	if len(line) == 0 {
		return
	}
	if e := ParseEntry(string(line)); f.Keeps(e) {
		*kept = append(*kept, e)
	}
}

// Follow hands to each the entries f keeps as they're written to the log at
// path, from offset on, until ctx is done or each fails. A line is handed
// once it ends, with its newline. A log that shrinks (cut by hand) is read
// again from its start.
func Follow(ctx context.Context, path string, offset int64, f Filter, each func(Entry) error) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	// The folder, not the file: a log not written yet, or replaced, is still seen.
	if err := w.Add(filepath.Dir(path)); err != nil {
		return err
	}
	var pending []byte // a line started, not ended yet
	read := func() error {
		file, err := os.Open(path) //nolint:gosec // G304: the repo's own log
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return err
		}
		if info.Size() < offset {
			offset, pending = 0, nil
		}
		if info.Size() == offset {
			return nil
		}
		b := make([]byte, info.Size()-offset)
		read, err := file.ReadAt(b, offset)
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		offset += int64(read)
		pending = append(pending, b[:read]...)
		for {
			i := bytes.IndexByte(pending, '\n')
			if i < 0 {
				return nil
			}
			line := string(pending[:i])
			pending = pending[i+1:]
			if line == "" {
				continue
			}
			if e := ParseEntry(line); f.Keeps(e) {
				if err := each(e); err != nil {
					return err
				}
			}
		}
	}
	// What was written between Read and the watch's start.
	if err := read(); err != nil {
		return err
	}
	// A poll beside the watch, for a file system that doesn't report writes
	// (some network and container mounts).
	poll := time.NewTicker(time.Second)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-w.Errors:
			return err
		case ev := <-w.Events:
			if filepath.Clean(ev.Name) != filepath.Clean(path) {
				continue
			}
		case <-poll.C:
		}
		if err := read(); err != nil {
			return err
		}
	}
}
