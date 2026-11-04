// Package pretty draws a command's result in the pretty format, for a person
// (docs/designs/cli-pretty.md): a view built from the result's shape by the
// conventions, adjusted by its fields' pretty tags, then drawn for a
// terminal.
package pretty

import (
	"encoding/csv"
	"fmt"
	"io"
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/holistics/anfra/internal/cliui"
)

// Render writes v, a command's result, in the pretty format, for a terminal
// of width (0: not a terminal, so nothing is cut). Colors follow w: on a
// terminal only, not with NO_COLOR.
func Render(w io.Writer, v any, width int) error {
	doc := buildPrettyDoc(reflect.ValueOf(v))
	r := prettyRenderer{styles: cliui.NewTerminalStyles(w), width: width}
	_, err := io.WriteString(w, r.doc(doc))
	return err
}

// The view model: what to show, decided once, whatever draws it.

// prettyDoc is a result's view: its message first, its hints last, its
// other fields between.
type prettyDoc struct {
	message string
	body    []prettyEntry
	hints   []string
}

// prettyEntry is a field: its label, and its value laid out one way.
type prettyEntry struct {
	label string
	value prettyValue
}

type prettyValueKind int

const (
	prettyText    prettyValueKind = iota // one line: cell
	prettyLines                          // several lines, aligned under the first: lines
	prettyBlock                          // text under its label: lines
	prettySection                        // a nested object: entries
	prettyList                           // a list of objects: table, else items
	prettyRows                           // a list of lists: rows
)

type prettyValue struct {
	kind    prettyValueKind
	cell    prettyCell
	lines   []prettyCell
	entries []prettyEntry
	table   *prettyTable   // nil when the list can't be a table
	items   []prettyItem   // the list as blocks
	rows    [][]prettyCell // a list of lists
}

// prettyItem is a list's object as a block: its first scalar as its heading.
type prettyItem struct {
	heading string
	entries []prettyEntry
}

type prettyTable struct {
	header   []string
	rows     [][]prettyCell
	truncate int // the column a hint lets be cut, or -1
}

// prettyCell is a value on one line: spans, each with its style.
type prettyCell []prettySpan

type prettySpan struct {
	text  string
	style prettyStyle
}

type prettyStyle int

const (
	stylePlain prettyStyle = iota
	styleMuted             // <none>
	styleGood
	styleWarn
	styleBad
	styleLink
)

func (c prettyCell) plain() string {
	var b strings.Builder
	for _, s := range c {
		b.WriteString(s.text)
	}
	return b.String()
}

func (c prettyCell) width() int { return ansi.StringWidth(c.plain()) }

// prettyField is a struct field as the view sees it.
type prettyField struct {
	name     string // its json name
	label    string
	omit     bool // omitempty or omitzero: an empty one is left out of key/values
	block    bool
	tree     bool
	moment   bool
	truncate bool
	tones    map[string]prettyStyle
	value    reflect.Value
}

// prettyFields are v's fields, embedded structs inlined as encoding/json
// does, hidden ones left out.
func prettyFields(v reflect.Value) []prettyField {
	var out []prettyField
	t := v.Type()
	for i := range t.NumField() {
		sf := t.Field(i)
		if !sf.IsExported() {
			continue
		}
		name, jsonOpts, _ := strings.Cut(sf.Tag.Get("json"), ",")
		if name == "-" || sf.Tag.Get("pretty") == "-" {
			continue
		}
		if sf.Anonymous && name == "" && deref(sf.Type).Kind() == reflect.Struct {
			if fv := indirect(v.Field(i)); fv.IsValid() {
				out = append(out, prettyFields(fv)...)
			}
			continue
		}
		if name == "" {
			name = sf.Name
		}
		f := prettyField{name: name, label: titleCase(name), value: v.Field(i),
			omit: strings.Contains(jsonOpts, "omitempty") || strings.Contains(jsonOpts, "omitzero")}
		label, opts, _ := strings.Cut(sf.Tag.Get("pretty"), ",")
		if label != "" {
			f.label = label
		}
		for opt := range strings.SplitSeq(opts, ",") {
			key, val, _ := strings.Cut(opt, "=")
			switch key {
			case "block":
				f.block = true
			case "tree":
				f.tree = true
			case "moment":
				f.moment = true
			case "truncate":
				f.truncate = true
			case "tone":
				f.tones = map[string]prettyStyle{}
				for pair := range strings.FieldsSeq(val) {
					value, tone, _ := strings.Cut(pair, ":")
					f.tones[value] = map[string]prettyStyle{"good": styleGood, "warn": styleWarn, "bad": styleBad}[tone]
				}
			}
		}
		out = append(out, f)
	}
	return out
}

// buildPrettyDoc is the view of result v: a top-level message and hints
// taken out, the rest its body.
func buildPrettyDoc(v reflect.Value) prettyDoc {
	var doc prettyDoc
	var body []prettyField
	for _, f := range prettyFields(indirect(v)) {
		switch f.name {
		case "message":
			doc.message = f.value.String()
		case "hints":
			for i := range f.value.Len() {
				doc.hints = append(doc.hints, f.value.Index(i).String())
			}
		default:
			body = append(body, f)
		}
	}
	doc.body = prettyEntries(body)
	return doc
}

// prettyEntries are fields as entries: an empty one marked omitempty left
// out, as the YAML leaves it out.
func prettyEntries(fields []prettyField) []prettyEntry {
	var out []prettyEntry
	for _, f := range fields {
		if f.omit && isEmpty(f.value) {
			continue
		}
		out = append(out, prettyEntry{label: f.label, value: prettyValueOf(f)})
	}
	return out
}

// prettyValueOf lays out a field's value by its shape: the conventions.
func prettyValueOf(f prettyField) prettyValue {
	v := indirect(f.value)
	if !v.IsValid() || isEmpty(v) {
		return prettyValue{kind: prettyText, cell: noneCell}
	}
	switch {
	case isScalar(v.Type()):
		if s, ok := v.Interface().(string); ok && (f.block || strings.Contains(s, "\n")) {
			return prettyValue{kind: prettyBlock, lines: textLines(s)}
		}
		return prettyValue{kind: prettyText, cell: scalarCell(f, v)}
	case v.Kind() == reflect.Struct:
		return prettyValue{kind: prettySection, entries: prettyEntries(prettyFields(v))}
	case v.Kind() == reflect.Slice || v.Kind() == reflect.Array:
		elem := deref(v.Type().Elem())
		switch {
		case isScalar(elem) && f.tree:
			var paths []string
			for i := range v.Len() {
				paths = append(paths, scalarCell(f, indirect(v.Index(i))).plain())
			}
			return prettyValue{kind: prettyBlock, lines: treeLines(paths, utf8Terminal())}
		case isScalar(elem):
			var lines []prettyCell
			for i := range v.Len() {
				lines = append(lines, scalarCell(f, indirect(v.Index(i))))
			}
			return prettyValue{kind: prettyLines, lines: lines}
		case elem.Kind() == reflect.Struct:
			return prettyListOf(v)
		case elem.Kind() == reflect.Slice:
			var rows [][]prettyCell
			for i := range v.Len() {
				var row []prettyCell
				for j := range v.Index(i).Len() {
					row = append(row, scalarCell(f, indirect(v.Index(i).Index(j))))
				}
				rows = append(rows, row)
			}
			return prettyValue{kind: prettyRows, rows: rows}
		}
	}
	return prettyValue{kind: prettyText, cell: prettyCell{{text: fmt.Sprint(v.Interface())}}}
}

// prettyListOf is a list of objects: always as blocks, and as a table too
// when every column's value fits in a cell.
func prettyListOf(v reflect.Value) prettyValue {
	out := prettyValue{kind: prettyList}
	tabular := true
	table := &prettyTable{truncate: -1}
	for i := range v.Len() {
		fields := prettyFields(indirect(v.Index(i)))
		var row []prettyCell
		for col, f := range fields {
			if i == 0 {
				table.header = append(table.header, strings.ToUpper(f.label))
				if f.truncate {
					table.truncate = col
				}
			}
			c, ok := cellOf(f)
			tabular = tabular && ok
			row = append(row, c)
		}
		table.rows = append(table.rows, row)

		item := prettyItem{}
		for j, f := range fields {
			if isScalar(deref(f.value.Type())) && !isEmpty(f.value) {
				item.heading = scalarCell(f, indirect(f.value)).plain()
				fields = append(fields[:j:j], fields[j+1:]...)
				break
			}
		}
		item.entries = prettyEntries(fields)
		out.items = append(out.items, item)
	}
	if tabular {
		out.table = table
	}
	return out
}

// cellOf is a field's value in a table cell, and whether it fits in one: a
// scalar, a list of scalars (a CSV row), an object of scalars, or a list of
// one such object (its values, space-separated).
func cellOf(f prettyField) (prettyCell, bool) {
	v := indirect(f.value)
	if !v.IsValid() || isEmpty(v) {
		return noneCell, true
	}
	switch {
	case isScalar(v.Type()):
		return firstLine(scalarCell(f, v)), true
	case v.Kind() == reflect.Struct:
		return joinedCell(scalarsOf(v), ' ')
	case v.Kind() == reflect.Slice || v.Kind() == reflect.Array:
		elem := deref(v.Type().Elem())
		if isScalar(elem) {
			var cells []prettyCell
			for i := range v.Len() {
				cells = append(cells, scalarCell(f, indirect(v.Index(i))))
			}
			return joinedCell(cells, ',')
		}
		if elem.Kind() == reflect.Struct && v.Len() == 1 {
			return joinedCell(scalarsOf(indirect(v.Index(0))), ' ')
		}
	}
	return nil, false
}

// scalarsOf is object v's values as cells, if they're all scalars.
func scalarsOf(v reflect.Value) []prettyCell {
	var cells []prettyCell
	for _, f := range prettyFields(v) {
		fv := indirect(f.value)
		if fv.IsValid() && !isScalar(fv.Type()) {
			return nil
		}
		if fv.IsValid() && !isEmpty(fv) {
			cells = append(cells, scalarCell(f, fv))
		}
	}
	if cells == nil {
		return []prettyCell{}
	}
	return cells
}

// joinedCell is cells on one line, separated by sep, each quoted as a CSV
// field is where sep or a quote would make it ambiguous; nil cells: not
// joinable.
func joinedCell(cells []prettyCell, sep rune) (prettyCell, bool) {
	if cells == nil {
		return nil, false
	}
	var out prettyCell
	for i, c := range cells {
		if i > 0 {
			out = append(out, prettySpan{text: string(sep)})
		}
		c = firstLine(c)
		if quoted := csvField(c.plain(), sep); quoted != c.plain() {
			c = prettyCell{{text: quoted, style: c[0].style}}
		}
		out = append(out, c...)
	}
	return out, true
}

// csvField is s as a field of a CSV record separated by sep: quoted, its
// quotes doubled, only where CSV needs it.
func csvField(s string, sep rune) string {
	var b strings.Builder
	w := csv.NewWriter(&b)
	w.Comma = sep
	_ = w.Write([]string{s})
	w.Flush()
	return strings.TrimSuffix(b.String(), "\n")
}

// scalarCell is scalar v as f shows it: a tone, a moment, true/false.
func scalarCell(f prettyField, v reflect.Value) prettyCell {
	var text string
	switch x := v.Interface().(type) {
	case time.Time:
		if f.moment {
			text = relativeTime(x, time.Now())
		} else {
			text = x.Local().Format("2006-01-02 15:04")
		}
	case fmt.Stringer:
		text = x.String()
	default:
		if v.Kind() == reflect.String {
			text = v.String()
		} else {
			text = fmt.Sprint(v.Interface())
		}
	}
	text = cliui.SafeText(text)
	style := stylePlain
	if tone, ok := f.tones[text]; ok {
		style = tone
	} else if strings.HasPrefix(text, "https://") || strings.HasPrefix(text, "http://") {
		style = styleLink
	}
	return prettyCell{{text: text, style: style}}
}

// noneCell is an empty value: nil, "", an empty list.
var noneCell = prettyCell{{text: "<none>", style: styleMuted}}

// firstLine is c's first line, then … when it has more.
func firstLine(c prettyCell) prettyCell {
	plain := c.plain()
	first, _, more := strings.Cut(plain, "\n")
	if !more {
		return c
	}
	return prettyCell{{text: first + "…", style: c[0].style}}
}

func textLines(s string) []prettyCell {
	var out []prettyCell
	for l := range strings.SplitSeq(strings.TrimRight(s, "\n"), "\n") {
		out = append(out, prettyCell{{text: cliui.SafeText(l)}})
	}
	return out
}

// treeNode is a folder of a tree: its folders and its files, by name.
type treeNode struct {
	dirs  map[string]*treeNode
	files []string
}

// treeLines is paths drawn as tree draws them: each folder's folders first,
// then its files, each sorted; a folder with one entry on its parent's line
// (models/.gitkeep); the lines faint. Without utf8, in ASCII.
func treeLines(paths []string, utf8 bool) []prettyCell {
	root := &treeNode{dirs: map[string]*treeNode{}}
	for _, p := range paths {
		n := root
		parts := strings.Split(p, "/")
		for _, d := range parts[:len(parts)-1] {
			if n.dirs[d] == nil {
				n.dirs[d] = &treeNode{dirs: map[string]*treeNode{}}
			}
			n = n.dirs[d]
		}
		n.files = append(n.files, parts[len(parts)-1])
	}
	branch, last, pipe := "├── ", "└── ", "│   "
	if !utf8 {
		branch, last, pipe = "|-- ", "`-- ", "|   "
	}
	var out []prettyCell
	var walk func(n *treeNode, prefix string)
	walk = func(n *treeNode, prefix string) {
		type entry struct {
			name string
			dir  *treeNode
		}
		var entries []entry
		for _, d := range slices.Sorted(maps.Keys(n.dirs)) {
			name, dir := d+"/", n.dirs[d]
			// A folder of one entry joins it on one line.
			for len(dir.files)+len(dir.dirs) == 1 {
				if len(dir.files) == 1 {
					name, dir = name+dir.files[0], nil
					break
				}
				for sub, next := range dir.dirs {
					name, dir = name+sub+"/", next
				}
			}
			entries = append(entries, entry{name, dir})
		}
		for _, f := range slices.Sorted(slices.Values(n.files)) {
			entries = append(entries, entry{f, nil})
		}
		for i, e := range entries {
			mark, next := branch, pipe
			if i == len(entries)-1 {
				mark, next = last, "    "
			}
			out = append(out, prettyCell{{text: prefix + mark, style: styleMuted}, {text: e.name}})
			if e.dir != nil {
				walk(e.dir, prefix+next)
			}
		}
	}
	walk(root, "")
	return out
}

// utf8Terminal reports whether the locale is UTF-8, for tree's lines.
func utf8Terminal() bool {
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := os.Getenv(k); v != "" {
			v = strings.ToUpper(v)
			return strings.Contains(v, "UTF-8") || strings.Contains(v, "UTF8")
		}
	}
	return false
}

// relativeTime is t as a person says it, from now: today, yesterday, n days
// ago, or its date past a month.
func relativeTime(t, now time.Time) string {
	t, now = t.Local(), now.Local()
	day := func(x time.Time) time.Time { y, m, d := x.Date(); return time.Date(y, m, d, 0, 0, 0, 0, x.Location()) }
	days := int(day(now).Sub(day(t)).Hours() / 24)
	switch {
	case days == 0:
		return "today"
	case days == 1:
		return "yesterday"
	case days > 1 && days < 31:
		return fmt.Sprintf("%d days ago", days)
	}
	return t.Format("2 Jan 2006")
}

// acronyms are words a label writes in capitals.
var acronyms = map[string]bool{"id": true, "url": true, "sql": true, "aql": true, "aml": true, "api": true, "json": true, "yaml": true, "md": true}

// titleCase is a json name as a label: its words capitalized, acronyms in
// capitals (checked_at → Checked At, org_url → Org URL).
func titleCase(name string) string {
	words := strings.Split(name, "_")
	for i, w := range words {
		switch {
		case acronyms[w]:
			words[i] = strings.ToUpper(w)
		case w != "":
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

func isScalar(t reflect.Type) bool {
	t = deref(t)
	if t == reflect.TypeFor[time.Time]() {
		return true
	}
	switch t.Kind() {
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

// isEmpty is a value encoding/json leaves out as empty: nil, "", an empty list.
func isEmpty(v reflect.Value) bool {
	if !v.IsValid() {
		return true
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		return v.Len() == 0
	}
	if t, ok := v.Interface().(time.Time); ok {
		return t.IsZero()
	}
	return false
}

func deref(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// indirect is v through its pointers, invalid at a nil one.
func indirect(v reflect.Value) reflect.Value {
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

// The terminal renderer: the view drawn, aligned, in the cliui styles.

type prettyRenderer struct {
	styles cliui.TerminalStyles
	width  int // the terminal's; 0: not a terminal
}

func (r prettyRenderer) doc(d prettyDoc) string {
	var b strings.Builder
	if d.message != "" {
		b.WriteString(r.styles.Heading.Render(r.inline(d.message)) + "\n")
		if len(d.body) > 0 || len(d.hints) > 0 {
			b.WriteString("\n")
		}
	}
	r.entries(&b, d.body, 0, true)
	if len(d.hints) > 0 {
		if len(d.body) > 0 {
			b.WriteString("\n")
		}
		b.WriteString("Hints:\n") // a section standing apart, as its siblings
		for _, h := range d.hints {
			b.WriteString("  " + r.inline(h) + "\n")
		}
	}
	return b.String()
}

// entries draws entries at indent: a value on its label's line, anything
// bigger under its label. Values align within a run of consecutive key/value
// lines, as gofmt aligns a struct literal's fields: a nested section ends the
// run. At the top, a blank line sets apart what has rows (a table, a list of
// blocks, a text block); a section of key/values stays with its neighbours.
// A section's label is plain where the section stands apart, a blank line (or
// the document's edge) before and after it, as a heading; elsewhere faint, as
// its siblings' labels are, the indentation showing what it holds. Bold is the
// message's alone.
func (r prettyRenderer) entries(b *strings.Builder, entries []prettyEntry, indent int, top bool) {
	labelWidths := runLabelWidths(entries)
	pad := strings.Repeat(" ", indent)
	apart := func(i int) bool { // a blank line, or the edge, between entries i-1 and i
		return i == 0 || i == len(entries) || hasRows(entries[i].value.kind) || hasRows(entries[i-1].value.kind)
	}
	for i, e := range entries {
		if top && i > 0 && apart(i) {
			b.WriteString("\n")
		}
		labelWidth := labelWidths[i]
		label := e.label + ":"
		heading := r.styles.Muted.Render(label)
		if top && apart(i) && apart(i+1) {
			heading = label
		}
		switch e.value.kind {
		case prettyText:
			b.WriteString(pad + r.styles.Muted.Render(label) + spaces(labelWidth-ansi.StringWidth(label)+2) + r.cell(e.value.cell) + "\n")
		case prettyLines:
			for j, l := range e.value.lines {
				head := r.styles.Muted.Render(label) + spaces(labelWidth-ansi.StringWidth(label)+2)
				if j > 0 {
					head = spaces(labelWidth + 2)
				}
				b.WriteString(pad + head + r.cell(l) + "\n")
			}
		case prettyBlock:
			b.WriteString(pad + heading + "\n")
			for _, l := range e.value.lines {
				b.WriteString(pad + "  " + r.cell(l) + "\n")
			}
		case prettySection:
			b.WriteString(pad + heading + "\n")
			r.entries(b, e.value.entries, indent+2, false)
		case prettyList:
			b.WriteString(pad + heading + "\n")
			if t := e.value.table; t != nil && r.fits(t, indent+2) {
				r.table(b, t, indent+2)
				break
			}
			for j, it := range e.value.items {
				if j > 0 {
					b.WriteString("\n")
				}
				b.WriteString(pad + "  " + r.inline(it.heading) + ":" + "\n")
				r.entries(b, it.entries, indent+4, false)
			}
		case prettyRows:
			b.WriteString(pad + heading + "\n")
			r.table(b, &prettyTable{rows: e.value.rows, truncate: -1}, indent+2)
		}
	}
}

// keyValue reports whether a value of kind goes on its label's line.
func keyValue(kind prettyValueKind) bool { return kind == prettyText || kind == prettyLines }

// hasRows reports whether a value of kind has rows, set apart at the top.
func hasRows(kind prettyValueKind) bool {
	return kind == prettyList || kind == prettyRows || kind == prettyBlock
}

// runLabelWidths is, for each key/value entry, the width its run's labels
// align to: its longest label and colon.
func runLabelWidths(entries []prettyEntry) []int {
	widths := make([]int, len(entries))
	for start := 0; start < len(entries); {
		if !keyValue(entries[start].value.kind) {
			start++
			continue
		}
		end, width := start, 0
		for ; end < len(entries) && keyValue(entries[end].value.kind); end++ {
			width = max(width, ansi.StringWidth(entries[end].label)+1)
		}
		for i := start; i < end; i++ {
			widths[i] = width
		}
		start = end
	}
	return widths
}

// fits reports whether t fits the terminal at indent, cutting its
// truncatable column to no less than 8; it does when there's no terminal.
func (r prettyRenderer) fits(t *prettyTable, indent int) bool {
	if r.width == 0 {
		return true
	}
	widths := columnWidths(t)
	total := indent + 3*(len(widths)-1)
	for _, w := range widths {
		total += w
	}
	if total <= r.width {
		return true
	}
	return t.truncate >= 0 && total-widths[t.truncate]+8 <= r.width
}

func columnWidths(t *prettyTable) []int {
	n := len(t.header)
	for _, row := range t.rows {
		n = max(n, len(row))
	}
	widths := make([]int, n)
	for i, h := range t.header {
		widths[i] = ansi.StringWidth(h)
	}
	for _, row := range t.rows {
		for i, c := range row {
			widths[i] = max(widths[i], c.width())
		}
	}
	return widths
}

// table draws t at indent: columns 3 spaces apart, its header in bold, its
// truncatable column cut to the terminal's width.
func (r prettyRenderer) table(b *strings.Builder, t *prettyTable, indent int) {
	widths := columnWidths(t)
	if r.width > 0 && t.truncate >= 0 {
		total := indent + 3*(len(widths)-1)
		for _, w := range widths {
			total += w
		}
		if over := total - r.width; over > 0 {
			widths[t.truncate] = max(widths[t.truncate]-over, 8)
		}
	}
	pad := strings.Repeat(" ", indent)
	line := func(cells []string, plainWidths []int) {
		var parts []string
		for i, c := range cells {
			if i < len(cells)-1 {
				c += spaces(widths[i] - plainWidths[i])
			}
			parts = append(parts, c)
		}
		b.WriteString(pad + strings.TrimRight(strings.Join(parts, "   "), " ") + "\n")
	}
	if len(t.header) > 0 {
		cells := make([]string, len(t.header))
		ws := make([]int, len(t.header))
		for i, h := range t.header {
			cells[i], ws[i] = r.styles.Muted.Render(h), ansi.StringWidth(h)
		}
		line(cells, ws)
	}
	for _, row := range t.rows {
		cells := make([]string, len(row))
		ws := make([]int, len(row))
		for i, c := range row {
			if c.width() > widths[i] {
				c = prettyCell{{text: ansi.Truncate(c.plain(), widths[i], "…"), style: c[0].style}}
			}
			cells[i], ws[i] = r.cell(c), c.width()
		}
		line(cells, ws)
	}
}

// cell draws c's spans in their styles, code within plain text styled too.
// Its text is safe already: cells are made of SafeText.
func (r prettyRenderer) cell(c prettyCell) string {
	var b strings.Builder
	for _, s := range c {
		switch s.style {
		case styleMuted:
			b.WriteString(r.styles.Muted.Render(s.text))
		case styleGood:
			b.WriteString(r.styles.Success.Render(s.text))
		case styleWarn:
			b.WriteString(r.styles.Warning.Render(s.text))
		case styleBad:
			b.WriteString(r.styles.Error.Render(s.text))
		case styleLink:
			b.WriteString(r.styles.Link.Render(s.text))
		default:
			b.WriteString(r.code(s.text))
		}
	}
	return b.String()
}

// inline is untrusted text, a message or a hint, made safe, its code styled.
func (r prettyRenderer) inline(text string) string { return r.code(cliui.SafeText(text)) }

// code is text with its `code` in the Code style, and without its
// backquotes, when there are styles; as written, when there aren't.
func (r prettyRenderer) code(text string) string {
	if r.styles.Code.Render("x") == "x" { // no styles: off a terminal, or NO_COLOR
		return text
	}
	parts := strings.Split(text, "`")
	if len(parts)%2 == 0 { // an unpaired backquote: as written
		return text
	}
	for i := 1; i < len(parts); i += 2 {
		parts[i] = r.styles.Code.Render(parts[i])
	}
	return strings.Join(parts, "")
}

func spaces(n int) string { return strings.Repeat(" ", max(n, 0)) }
