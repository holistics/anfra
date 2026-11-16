package pretty

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/shared/jsonkit"
)

// views are named views, ad hoc: each lays out one type of value no
// convention lays out well, a hint naming it (pretty:",view=timings"). One
// place for them, each named for its type of value rather than a command, so
// every result holding one shows it alike
// (docs/designs/cli-pretty.md#a-commands-own-view). A view answers false for
// a value it can't show, which then shows by its shape.
var views = map[string]func(v reflect.Value) (prettyValue, bool){
	"timings":     timingsView,
	"log_entries": logEntriesView,
}

// timingsView is a run's timings (command.Timings: steps, each when it
// started, and when the run ended) as how long each step took, which they
// record only by its start: a step lasts until the next at its level or
// above starts, or the run ends. A child step (<parent>.<child>) is nested
// under its parent; a step run again is shown again; all faint, the detail.
// Then the run's total, plain: what a person reads first.
func timingsView(v reflect.Value) (prettyValue, bool) {
	steps := indirect(fieldNamed(v, "steps"))
	end := indirect(fieldNamed(v, "end_ms"))
	if !steps.IsValid() || steps.Kind() != reflect.Slice || !end.IsValid() || !end.CanInt() {
		return prettyValue{}, false
	}
	type step struct {
		name  string
		depth int
		start int64
	}
	var all []step
	for i := range steps.Len() {
		s := indirect(steps.Index(i))
		name, start := indirect(fieldNamed(s, "step")), indirect(fieldNamed(s, "start_ms"))
		if !name.IsValid() || !start.IsValid() || !start.CanInt() {
			return prettyValue{}, false
		}
		all = append(all, step{name: name.String(), depth: strings.Count(name.String(), "."), start: start.Int()})
	}
	type line struct {
		label string
		ms    int64
	}
	var lines []line
	width := len("total")
	for i, s := range all {
		until := end.Int()
		for _, next := range all[i+1:] {
			if next.depth <= s.depth {
				until = next.start
				break
			}
		}
		label := strings.Repeat("  ", s.depth) + s.name[strings.LastIndex(s.name, ".")+1:]
		lines = append(lines, line{label, until - s.start})
		width = max(width, len(label))
	}
	ms := func(n int64) string { return fmt.Sprintf("%d ms", n) }
	msWidth := len(ms(end.Int()))
	for _, l := range lines {
		msWidth = max(msWidth, len(ms(l.ms)))
	}
	var out []prettyCell
	for _, l := range lines {
		out = append(out, prettyCell{{text: l.label + spaces(width-len(l.label)+3+msWidth-len(ms(l.ms))) + ms(l.ms), style: styleMuted}})
	}
	total := "total" + spaces(width-len("total")+3+msWidth-len(ms(end.Int()))) + ms(end.Int())
	out = append(out, prettyCell{{text: total}})
	return prettyValue{kind: prettyBlock, lines: out}, true
}

// logEntriesView is a repo's log entries (logging.Entry), each on its own, so
// that one followed as it's written is laid out as the ones before it: its
// time, level, message and event on a line, then each of its other fields on
// a line under the message; or a sidecar's line, faint. The level is
// colored, ERROR red, WARN yellow, DEBUG faint, so the eye finds what went
// wrong; the event is accented, to scan by. Departs from the result in
// leaving out pid, trace_id and span_id, which a person reading never needs
// and a program reads from --json.
func logEntriesView(v reflect.Value) (prettyValue, bool) {
	if v.Kind() != reflect.Slice {
		return prettyValue{}, false
	}
	var lines []prettyCell
	now := time.Now()
	for i := range v.Len() {
		lines = append(lines, logEntryLines(indirect(v.Index(i)), now)...)
	}
	return prettyValue{kind: prettyBlock, lines: lines}, true
}

// logHiddenAttrs are the fields logEntriesView leaves out.
var logHiddenAttrs = []string{"pid", "trace_id", "span_id"}

// logLevelStyles are each level's style; INFO's is plain.
var logLevelStyles = map[string]prettyStyle{"ERROR": styleBad, "WARN": styleWarn, "DEBUG": styleMuted}

func logEntryLines(e reflect.Value, now time.Time) []prettyCell {
	str := func(name string) string {
		if f := indirect(fieldNamed(e, name)); f.IsValid() && f.Kind() == reflect.String {
			return f.String()
		}
		return ""
	}
	at := ""
	if f := indirect(fieldNamed(e, "ts")); f.IsValid() {
		if t, ok := f.Interface().(time.Time); ok && !t.IsZero() {
			at = logTime(t.Local(), now)
		}
	}
	timeSpan := prettySpan{text: at + spaces(len("15:04:05.000")-len(at)) + "  ", style: styleMuted}
	if line := str("line"); line != "" {
		return []prettyCell{{timeSpan, {text: cliui.SafeText(line), style: styleMuted}}}
	}
	level := str("level")
	head := prettyCell{timeSpan, {text: level + spaces(5-len(level)) + "  ", style: logLevelStyles[level]}, {text: cliui.SafeText(str("msg"))}}
	if event := str("event"); event != "" {
		head = append(head, prettySpan{text: "  " + cliui.SafeText(event), style: styleAccent})
	}
	if service := str("service"); service != "" && service != "anfra-host" {
		head = append(head, prettySpan{text: "  (" + cliui.SafeText(service) + ")", style: styleMuted})
	}
	lines := []prettyCell{head}
	under := spaces(len(timeSpan.text) + 5 + 2)
	attrs := indirect(fieldNamed(e, "attrs"))
	if !attrs.IsValid() || attrs.Kind() != reflect.Map {
		return lines
	}
	var names []string
	for _, k := range attrs.MapKeys() {
		if !slices.Contains(logHiddenAttrs, k.String()) {
			names = append(names, k.String())
		}
	}
	slices.Sort(names)
	for _, name := range names {
		value := indirect(attrs.MapIndex(reflect.ValueOf(name)))
		text := ""
		if value.IsValid() {
			if s, ok := value.Interface().(string); ok {
				text = s
			} else {
				b, _ := jsonkit.Marshal(value.Interface())
				text = string(b)
			}
		}
		for j, l := range strings.Split(text, "\n") { // a stack, a line each, under its name
			label := name + ": "
			if j > 0 {
				label = spaces(len(label))
			}
			lines = append(lines, prettyCell{{text: under + cliui.SafeText(label+l), style: styleMuted}})
		}
	}
	return lines
}

// logTime is when an entry was written: the time alone today, the date too
// before; its milliseconds, unless it was only written to the second (a
// sidecar's own line), which .000 would make look earlier than it was.
func logTime(t, now time.Time) string {
	layout := "15:04:05.000"
	if t.Nanosecond() == 0 {
		layout = "15:04:05"
	}
	if y, m, d := t.Date(); y != now.Year() || m != now.Month() || d != now.Day() {
		layout = "2006-01-02 " + layout
	}
	return t.Format(layout)
}
