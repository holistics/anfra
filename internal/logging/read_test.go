package logging

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A line is a record of anfra's or anfra-node's, its level in capitals and
// its other fields kept but repo; or a sidecar's, with the time Go's log
// starts it with; or any other line, as it is.
func TestParseEntry(t *testing.T) {
	host := ParseEntry(`{"ts":"2026-11-16T12:11:57.493+07:00","level":"WARN","msg":"Query ran without the cache","service":"anfra-host","repo":"r","pid":7,"event":"query_cache.unavailable","error":"start canal-lake: boom"}`)
	if host.Level != "WARN" || host.Message != "Query ran without the cache" || host.Event != "query_cache.unavailable" || host.Service != "anfra-host" ||
		host.Attrs["error"] != "start canal-lake: boom" || host.Attrs["pid"] != float64(7) || host.Attrs["repo"] != nil || host.Time.IsZero() {
		t.Errorf("anfra's record: %+v", host)
	}
	if node := ParseEntry(`{"level":"info","msg":"sidecar.listening","service":"anfra-node","ts":"2026-10-07T12:32:49.370Z"}`); node.Level != "INFO" || node.Message != "sidecar.listening" || node.Event != "" {
		t.Errorf("anfra-node's record: %+v", node)
	}
	canal := ParseEntry("2026/11/14 09:53:03 starting Canal Query")
	if canal.Line != "starting Canal Query" || canal.Time != time.Date(2026, 11, 14, 9, 53, 3, 0, time.Local) || canal.Level != "" {
		t.Errorf("a sidecar's line: %+v", canal)
	}
	for _, line := range []string{"panic: boom", `{"not":"a record"}`, "{broken"} {
		if e := ParseEntry(line); e.Line != line || !e.Time.IsZero() {
			t.Errorf("%q: %+v", line, e)
		}
	}
}

func TestFilter(t *testing.T) {
	at := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	warn := Entry{Time: at, Level: "WARN", Event: "query_cache.unavailable"}
	info := Entry{Time: at.Add(-time.Hour), Level: "INFO", Event: "query_cachex.other"}
	line := Entry{Time: at, Line: "starting Canal Query"}
	for _, tc := range []struct {
		name string
		f    Filter
		want []bool // warn, info, line
	}{
		{"none", Filter{}, []bool{true, true, true}},
		{"a level, and those above", Filter{Level: "warn"}, []bool{true, false, false}},
		{"an event, and those under it", Filter{Events: []string{"query_cache"}}, []bool{true, false, false}},
		{"an event by its whole name", Filter{Events: []string{"query_cachex.other"}}, []bool{false, true, false}},
		{"since", Filter{Since: at.Add(-time.Minute)}, []bool{true, false, true}},
	} {
		for i, e := range []Entry{warn, info, line} {
			if got := tc.f.Keeps(e); got != tc.want[i] {
				t.Errorf("%s: entry %d kept %v, want %v", tc.name, i, got, tc.want[i])
			}
		}
	}
}

// The last entries come from the end back, across blocks, oldest first; a
// last line still being written waits for Follow, which carries on where Read
// stopped and hands each line once it ends.
func TestReadAndFollow(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	var b strings.Builder
	for i := range 3000 { // over readBlock
		fmt.Fprintf(&b, `{"ts":"2026-11-16T12:00:00Z","level":"%s","msg":"m%d","event":"e.%d"}`+"\n", map[bool]string{true: "WARN", false: "INFO"}[i%10 == 0], i, i)
	}
	b.WriteString(`{"ts":"2026-11-16T12:00:00Z","level":"INFO","msg":"half`) // being written
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	last, offset, err := Read(path, Filter{}, 3)
	if err != nil || len(last) != 3 || last[0].Message != "m2997" || last[2].Message != "m2999" {
		t.Fatalf("the last 3: %+v, %v", last, err)
	}
	if warns, _, _ := Read(path, Filter{Level: "warn"}, 2); len(warns) != 2 || warns[0].Message != "m2980" || warns[1].Message != "m2990" {
		t.Errorf("the last 2 warnings: %+v", warns)
	}
	if all, _, _ := Read(path, Filter{}, 0); len(all) != 3000 || all[0].Message != "m0" {
		t.Errorf("every entry: %d, first %+v", len(all), all[0])
	}
	if none, end, err := Read(filepath.Join(t.TempDir(), "none.log"), Filter{}, 5); none != nil || end != 0 || err != nil {
		t.Errorf("no log: %v, %d, %v", none, end, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	got := make(chan Entry, 10)
	done := make(chan error, 1)
	go func() {
		done <- Follow(ctx, path, offset, Filter{}, func(e Entry) error { got <- e; return nil })
	}()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // G304: the test's own file
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`", "service":"anfra-host"}` + "\n2026/11/16 12:00:01 shutting down Canal Lake\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	for _, want := range []string{"half", "shutting down Canal Lake"} {
		select {
		case e := <-got:
			if e.Message+e.Line != want {
				t.Errorf("followed %+v, want %q", e, want)
			}
		case <-ctx.Done():
			t.Fatalf("waited for %q", want)
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Errorf("Follow: %v", err)
	}
}
