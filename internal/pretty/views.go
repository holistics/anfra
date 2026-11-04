package pretty

import (
	"fmt"
	"reflect"
	"strings"
)

// views are named views, for a value no convention lays out well, a hint
// naming one (pretty:",view=timings"): one place for them, each of a type of
// value rather than of a command, so every result holding one shows it alike
// (docs/designs/cli-pretty.md#a-commands-own-view). A view answers false for
// a value it can't show, which then shows by its shape.
var views = map[string]func(v reflect.Value) (prettyValue, bool){
	"timings": timingsView,
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
