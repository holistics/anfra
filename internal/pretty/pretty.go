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
	"math"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/shared/jsonkit"
)

// Render writes v, a command's result, in the pretty format, for a terminal
// of width (0: not a terminal, so nothing is cut). Colors follow w: on a
// terminal only, not with NO_COLOR.
func Render(w io.Writer, v any, width int) error {
	return RenderStyled(w, v, width, cliui.NewTerminalStyles(w))
}

// RenderStyled is Render in styles, the terminal's it's bound for: for a
// view written into a buffer first, to be placed (an error's details, under
// its headline), which isn't a terminal itself.
func RenderStyled(w io.Writer, v any, width int, styles cliui.TerminalStyles) error {
	doc := buildPrettyDoc(reflect.ValueOf(v))
	r := prettyRenderer{styles: styles, width: width}
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
	labels  []string       // the rows' values' labels, from a hint; none without one
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
	name      string // its json name
	label     string
	omit      bool // omitempty or omitzero: an empty one is left out of key/values
	block     bool
	tree      bool
	moment    bool
	truncate  bool
	rowLabels string // rows: the path, from the result's top, of their values' labels (columns.label)
	view      string // a named view of the value (views), rather than its shape's
	tones     map[string]prettyStyle
	value     reflect.Value
	root      reflect.Value // the result, for a path from its top
}

// Object is an object read from JSON or YAML with its keys in order (an
// error's details, say), laid out as a struct is, in that order. A map is
// too, its keys sorted, since it keeps no order.
type Object struct {
	Keys   []string
	Values map[string]any
}

// prettyFields are v's fields, embedded structs inlined as encoding/json
// does, and those a hint inlines; hidden ones left out. root is the result
// they're part of. An object decoded from JSON (a map) has its keys as
// fields, sorted, since a map keeps no order.
func prettyFields(v, root reflect.Value) []prettyField {
	var out []prettyField
	if v.CanInterface() && v.Type() == reflect.TypeFor[Object]() {
		o := v.Interface().(Object)
		for _, k := range o.Keys {
			out = append(out, prettyField{name: k, label: sentenceCase(k), value: reflect.ValueOf(o.Values).MapIndex(reflect.ValueOf(k)), root: root})
		}
		return out
	}
	if v.Kind() == reflect.Map {
		keys := v.MapKeys()
		slices.SortFunc(keys, func(a, b reflect.Value) int { return strings.Compare(a.String(), b.String()) })
		for _, k := range keys {
			// A map's keys are data (names, addresses), not field names:
			// shown as they are.
			out = append(out, prettyField{name: k.String(), label: k.String(), value: v.MapIndex(k), root: root})
		}
		return out
	}
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
				out = append(out, prettyFields(fv, root)...)
			}
			continue
		}
		if name == "" {
			name = sf.Name
		}
		f := prettyField{name: name, label: sentenceCase(name), value: v.Field(i), root: root,
			omit: strings.Contains(jsonOpts, "omitempty") || strings.Contains(jsonOpts, "omitzero")}
		inline := false
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
			case "rowlabels":
				f.rowLabels = val
			case "view":
				f.view = val
			case "inline":
				inline = true
			case "tone":
				f.tones = map[string]prettyStyle{}
				for pair := range strings.FieldsSeq(val) {
					value, tone, _ := strings.Cut(pair, ":")
					f.tones[value] = map[string]prettyStyle{"good": styleGood, "warn": styleWarn, "bad": styleBad}[tone]
				}
			}
		}
		// An inlined object's shown fields are its parent's, as encoding/json
		// inlines an embedded one: the rows of a query's result, under no
		// heading of their own.
		if fv := indirect(f.value); inline && fv.IsValid() && (fv.Kind() == reflect.Struct || fv.Kind() == reflect.Map) {
			out = append(out, prettyFields(fv, root)...)
			continue
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
	for _, f := range prettyFields(indirect(v), indirect(v)) {
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

// prettyValueOf lays out a field's value by its shape: the conventions; or
// by the named view a hint gives it.
func prettyValueOf(f prettyField) prettyValue {
	if view, ok := views[f.view]; ok {
		if pv, ok := view(indirect(f.value)); ok {
			return pv
		}
	}
	v := decodedJSON(indirect(f.value))
	if !v.IsValid() || isEmpty(v) {
		return prettyValue{kind: prettyText, cell: noneCell}
	}
	switch {
	case isScalar(v.Type()):
		if s, ok := v.Interface().(string); ok && (f.block || strings.Contains(s, "\n")) {
			return prettyValue{kind: prettyBlock, lines: textLines(s)}
		}
		return prettyValue{kind: prettyText, cell: scalarCell(f, v)}
	case v.Kind() == reflect.Struct || v.Kind() == reflect.Map:
		return prettyValue{kind: prettySection, entries: prettyEntries(prettyFields(v, f.root))}
	case v.Kind() == reflect.Slice || v.Kind() == reflect.Array:
		elem := shapeOf(v)
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
		case elem.Kind() == reflect.Struct || elem.Kind() == reflect.Map:
			if lines, ok := singleFieldLines(v, f.root); ok {
				return prettyValue{kind: prettyLines, lines: lines}
			}
			return prettyListOf(v, f.root)
		case elem.Kind() == reflect.Slice:
			var rows [][]prettyCell
			for i := range v.Len() {
				var row []prettyCell
				rv := indirect(v.Index(i))
				for j := range rv.Len() {
					c := indirect(rv.Index(j))
					switch {
					case !c.IsValid():
						row = append(row, noneCell) // a null
					case isScalar(c.Type()):
						row = append(row, firstLine(scalarCell(f, c)))
					default: // a list or a map in a cell: as its JSON
						b, _ := jsonkit.Marshal(c.Interface())
						row = append(row, prettyCell{{text: cliui.SafeText(string(b))}})
					}
				}
				rows = append(rows, row)
			}
			return prettyValue{kind: prettyRows, rows: rows, labels: pathStrings(f.root, f.rowLabels)}
		}
	}
	return prettyValue{kind: prettyText, cell: prettyCell{{text: fmt.Sprint(v.Interface())}}}
}

// decodedJSON is v, when it's raw JSON (a jsontext.Value, a canal-query row),
// as the value it holds: laid out as that value would be. A list of raw JSON
// values is a list of the values they hold.
func decodedJSON(v reflect.Value) reflect.Value {
	if !v.IsValid() {
		return v
	}
	t := v.Type()
	switch {
	case isRawJSON(t):
		var x any
		if jsonkit.Unmarshal(v.Bytes(), &x) != nil {
			return v
		}
		return reflect.ValueOf(&x).Elem()
	case t.Kind() == reflect.Slice && isRawJSON(t.Elem()):
		out := make([]any, v.Len())
		for i := range v.Len() {
			if d := indirect(decodedJSON(v.Index(i))); d.IsValid() {
				out[i] = d.Interface()
			}
		}
		return reflect.ValueOf(out)
	}
	return v
}

// isRawJSON reports whether t is raw JSON: bytes, which encoding/json would
// otherwise write as base64, a result never holds.
func isRawJSON(t reflect.Type) bool {
	return t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8
}

// shapeOf is the type of list v's elements, or, for a list of anything (as
// JSON decodes), the type its elements share: a list, when every one is.
func shapeOf(v reflect.Value) reflect.Type {
	elem := deref(v.Type().Elem())
	if elem.Kind() != reflect.Interface || v.Len() == 0 {
		return elem
	}
	var shared reflect.Type
	for i := range v.Len() {
		e := indirect(v.Index(i))
		if !e.IsValid() {
			continue
		}
		t := e.Type()
		switch t.Kind() {
		case reflect.Slice:
			t = reflect.TypeFor[[]any]()
		case reflect.Map:
			t = reflect.TypeFor[map[string]any]()
		}
		if shared != nil && shared != t {
			return elem
		}
		shared = t
	}
	if shared == nil {
		return elem
	}
	return shared
}

// pathStrings is the strings at path in root, dot-separated JSON names
// through objects and lists (columns.label: each column's label); none when
// path is "" or leads nowhere.
func pathStrings(root reflect.Value, path string) []string {
	if path == "" {
		return nil
	}
	vals := []reflect.Value{indirect(root)}
	for name := range strings.SplitSeq(path, ".") {
		var next []reflect.Value
		for _, v := range vals {
			if v.Kind() == reflect.Slice {
				for i := range v.Len() {
					next = append(next, indirect(fieldNamed(indirect(v.Index(i)), name)))
				}
			} else {
				next = append(next, indirect(fieldNamed(v, name)))
			}
		}
		vals = next
	}
	var out []string
	for _, v := range vals {
		switch {
		case !v.IsValid():
		case v.Kind() == reflect.Slice:
			for i := range v.Len() {
				out = append(out, fmt.Sprint(indirect(v.Index(i)).Interface()))
			}
		default:
			out = append(out, fmt.Sprint(v.Interface()))
		}
	}
	return out
}

// fieldNamed is struct v's field whose JSON name is name, embedded ones
// searched too; invalid when there's none.
func fieldNamed(v reflect.Value, name string) reflect.Value {
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return reflect.Value{}
	}
	t := v.Type()
	for i := range t.NumField() {
		sf := t.Field(i)
		n, _, _ := strings.Cut(sf.Tag.Get("json"), ",")
		if n == name || n == "" && sf.Name == name {
			return v.Field(i)
		}
		if sf.Anonymous && n == "" {
			if fv := fieldNamed(indirect(v.Field(i)), name); fv.IsValid() {
				return fv
			}
		}
	}
	return reflect.Value{}
}

// prettyListOf is a list of objects: always as blocks, and as a table too
// when every column's value fits in a cell.
func prettyListOf(v, root reflect.Value) prettyValue {
	out := prettyValue{kind: prettyList}
	tabular := true
	table := &prettyTable{truncate: -1}
	for i := range v.Len() {
		fields := prettyFields(indirect(v.Index(i)), root)
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
			if fv := indirect(f.value); fv.IsValid() && isScalar(fv.Type()) && !isEmpty(fv) {
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

// singleFieldLines is a list of objects that each show one scalar field, as
// that field's values: a list of scalars, one a line. A table of one column
// would add only a header naming it.
func singleFieldLines(v, root reflect.Value) ([]prettyCell, bool) {
	var lines []prettyCell
	for i := range v.Len() {
		fields := prettyFields(indirect(v.Index(i)), root)
		if len(fields) != 1 {
			return nil, false
		}
		fv := indirect(fields[0].value)
		if !fv.IsValid() || !isScalar(fv.Type()) {
			return nil, false
		}
		lines = append(lines, scalarCell(fields[0], fv))
	}
	return lines, len(lines) > 0
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
	case v.Kind() == reflect.Struct || v.Kind() == reflect.Map:
		return joinedCell(scalarsOf(v), ' ')
	case v.Kind() == reflect.Slice || v.Kind() == reflect.Array:
		elem := shapeOf(v)
		if isScalar(elem) {
			var cells []prettyCell
			for i := range v.Len() {
				cells = append(cells, scalarCell(f, indirect(v.Index(i))))
			}
			return joinedCell(cells, ',')
		}
		if (elem.Kind() == reflect.Struct || elem.Kind() == reflect.Map) && v.Len() == 1 {
			return joinedCell(scalarsOf(indirect(v.Index(0))), ' ')
		}
	}
	return nil, false
}

// scalarsOf is object v's values as cells, if they're all scalars.
func scalarsOf(v reflect.Value) []prettyCell {
	var cells []prettyCell
	for _, f := range prettyFields(v, reflect.Value{}) {
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
	// Rounded, as midnights a DST change apart are 23 or 25 hours apart.
	days := int(math.Round(day(now).Sub(day(t)).Hours() / 24))
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

// sentenceCase is a json name as a label: its first word capitalized, the
// rest as they are, acronyms in capitals (checked_at → Checked at, org_url →
// Org URL).
func sentenceCase(name string) string {
	words := strings.Split(name, "_")
	for i, w := range words {
		switch {
		case acronyms[w]:
			words[i] = strings.ToUpper(w)
		case i == 0 && w != "":
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
		b.WriteString(r.styles.Key.Render("Hints:") + "\n") // a key, as its siblings
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
// Every label is a key, bright blue as YAML's are, a section's as a value's:
// siblings alike, the indentation and the blank lines showing what a section
// holds. Bold is the message's alone.
func (r prettyRenderer) entries(b *strings.Builder, entries []prettyEntry, indent int, top bool) {
	labelWidths := runLabelWidths(entries)
	under := r.overflowingRuns(entries, labelWidths, indent)
	pad := strings.Repeat(" ", indent)
	apart := func(i int) bool { // a blank line, or the edge, between entries i-1 and i
		return i == 0 || i == len(entries) || setApart(entries[i].value.kind) || setApart(entries[i-1].value.kind)
	}
	for i, e := range entries {
		if top && i > 0 && apart(i) {
			b.WriteString("\n")
		}
		labelWidth := labelWidths[i]
		label := e.label + ":"
		heading := r.styles.Key.Render(label)
		if under[i] {
			// A run that doesn't fit the terminal: each value under its label.
			b.WriteString(pad + heading + "\n")
			lines := e.value.lines
			if e.value.kind == prettyText {
				lines = []prettyCell{e.value.cell}
			}
			for _, l := range lines {
				b.WriteString(pad + "  " + r.cell(l) + "\n")
			}
			continue
		}
		switch e.value.kind {
		case prettyText:
			b.WriteString(pad + heading + spaces(labelWidth-ansi.StringWidth(label)+2) + r.cell(e.value.cell) + "\n")
		case prettyLines:
			for j, l := range e.value.lines {
				head := heading + spaces(labelWidth-ansi.StringWidth(label)+2)
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
			r.grid(b, e.value.rows, e.value.labels, indent+2)
		}
	}
}

// keyValue reports whether a value of kind goes on its label's line.
func keyValue(kind prettyValueKind) bool { return kind == prettyText || kind == prettyLines }

// overflowingRuns marks the entries of each run of key/value lines that
// doesn't fit the terminal: one of its lines, its label and value side by
// side, is wider than the terminal. The whole run then puts each value under
// its label, so siblings stay alike. Not a terminal (width 0), none does.
func (r prettyRenderer) overflowingRuns(entries []prettyEntry, labelWidths []int, indent int) []bool {
	under := make([]bool, len(entries))
	if r.width <= 0 {
		return under
	}
	for start := 0; start < len(entries); {
		if !keyValue(entries[start].value.kind) {
			start++
			continue
		}
		end, overflows := start, false
		for ; end < len(entries) && keyValue(entries[end].value.kind); end++ {
			e := entries[end]
			cells := e.value.lines
			if e.value.kind == prettyText {
				cells = []prettyCell{e.value.cell}
			}
			for _, c := range cells {
				if indent+labelWidths[end]+2+c.width() > r.width {
					overflows = true
				}
			}
		}
		for i := start; i < end; i++ {
			under[i] = overflows
		}
		start = end
	}
	return under
}

// setApart reports whether a value of kind is set apart by a blank line at
// the top: what has rows, and a section, whose contents are its own.
func setApart(kind prettyValueKind) bool { return hasRows(kind) || kind == prettySection }

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

// table draws t at indent: columns 3 spaces apart, its header faint, a
// column
// of numbers right-aligned, its truncatable column cut to the terminal's
// width. A listing's table: rows of data are a grid.
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
	numeric := numericColumns(t.rows)
	line := func(cells []string, plainWidths []int) {
		var parts []string
		for i, c := range cells {
			switch {
			case numeric[i]:
				c = spaces(widths[i]-plainWidths[i]) + c
			case i < len(cells)-1:
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
			cells[i], ws[i] = r.styles.Muted.Render(h), ansi.StringWidth(h) // uppercase marks a header already
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

// numericColumns reports, for each column of rows, whether every value in it
// is a number, <none> aside: right-aligned, as numbers are read.
func numericColumns(rows [][]prettyCell) []bool {
	n := 0
	for _, row := range rows {
		n = max(n, len(row))
	}
	numeric := make([]bool, n)
	for i := range numeric {
		any := false
		numeric[i] = true
		for _, row := range rows {
			if i >= len(row) || row[i].plain() == noneCell.plain() {
				continue
			}
			if _, err := strconv.ParseFloat(row[i].plain(), 64); err != nil {
				numeric[i] = false
				break
			}
			any = true
		}
		numeric[i] = numeric[i] && any
	}
	return numeric
}

// grid draws rows, data, at indent: a bordered grid, as psql and duckdb draw
// one, since a value may hold spaces or be empty, and only a border says where
// a cell ends; its values' labels, when a hint gives them, as its header,
// faint; numbers right-aligned. Wider than the terminal, it's one block per
// row instead, a label and a value a line, as psql's expanded mode. Without a
// UTF-8 terminal, in ASCII.
func (r prettyRenderer) grid(b *strings.Builder, rows [][]prettyCell, labels []string, indent int) {
	t := &prettyTable{header: labels, rows: rows, truncate: -1}
	widths := columnWidths(t)
	total := indent + 1
	for _, w := range widths {
		total += w + 3
	}
	pad := strings.Repeat(" ", indent)
	if r.width > 0 && total > r.width {
		r.rowBlocks(b, rows, labels, pad)
		return
	}
	h, v, tl, tm, tr, ml, mm, mr, bl, bm, br := "─", "│", "┌", "┬", "┐", "├", "┼", "┤", "└", "┴", "┘"
	if !utf8Terminal() {
		h, v, tl, tm, tr, ml, mm, mr, bl, bm, br = "-", "|", "+", "+", "+", "+", "+", "+", "+", "+", "+"
	}
	border := func(left, mid, right string) {
		var parts []string
		for _, w := range widths {
			parts = append(parts, strings.Repeat(h, w+2))
		}
		b.WriteString(pad + r.styles.Muted.Render(left+strings.Join(parts, mid)+right) + "\n")
	}
	numeric := numericColumns(rows)
	line := func(cells []prettyCell, header bool) {
		b.WriteString(pad + r.styles.Muted.Render(v))
		for i, w := range widths {
			c := prettyCell{}
			if i < len(cells) {
				c = cells[i]
			}
			text := r.cell(c)
			if header { // faint, as a listing's header is: the columns' names, not a value
				text = r.styles.Muted.Render(c.plain())
			}
			gap := spaces(w - c.width())
			if numeric[i] && !header {
				text = gap + text
			} else {
				text += gap
			}
			b.WriteString(" " + text + " " + r.styles.Muted.Render(v))
		}
		b.WriteString("\n")
	}
	border(tl, tm, tr)
	if len(labels) > 0 {
		head := make([]prettyCell, len(labels))
		for i, l := range labels {
			head[i] = prettyCell{{text: cliui.SafeText(l)}}
		}
		line(head, true)
		border(ml, mm, mr)
	}
	for _, row := range rows {
		line(row, false)
	}
	border(bl, bm, br)
}

// rowBlocks draws rows one block a row, as psql's expanded mode: a faint
// "-[ ROW n ]-" line, then each value under its label, labels faint, numbered
// when no hint gives them.
func (r prettyRenderer) rowBlocks(b *strings.Builder, rows [][]prettyCell, labels []string, pad string) {
	width := 0
	label := func(i int) string {
		if i < len(labels) {
			return cliui.SafeText(labels[i])
		}
		return strconv.Itoa(i + 1)
	}
	for _, row := range rows {
		for i := range row {
			width = max(width, ansi.StringWidth(label(i)))
		}
	}
	for n, row := range rows {
		b.WriteString(pad + r.styles.Muted.Render(fmt.Sprintf("-[ ROW %d ]-", n+1)) + "\n")
		for i, c := range row {
			l := label(i)
			b.WriteString(pad + r.styles.Muted.Render(l) + spaces(width-ansi.StringWidth(l)+3) + r.cell(c) + "\n")
		}
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
