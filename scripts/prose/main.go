// Command prose collects the text anfra's Go code writes for its users to read
// into review tables: a folder per kind of text (errors, descriptions, help
// and other), and in it a YAML file per area of the code, of at most a
// review batch each. It's the extraction step of docs/todos/improve-proses.md.
//
//	go run ./scripts/prose [-force]
//
// It writes into tmp/prose/, which git ignores: the tables are worked on and
// reviewed locally, not committed. It refuses to replace a kind's folder
// that's already there, since its entries may hold triage and rewrites:
// -force replaces it.
//
// It parses the code rather than grepping it, so it joins "a" + "b", knows
// which field or call a string is in, and skips tests. A string found in
// several places is one entry, listing each.
package main

import (
	"cmp"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/goccy/go-yaml"
)

// The Go code users' text comes from.
var roots = []string{"cmd", "engine", "internal", "shared"}

// An entry is one piece of text, and the review fields filled in later.
type entry struct {
	Where   []string `yaml:"where"`
	Context string   `yaml:"context"`
	Now     string   `yaml:"now"`
	// Args are a format string's arguments, in order, as Go expressions.
	Args []string `yaml:"args,omitempty"`
	// Manual is text joined from a literal and an expression, which the apply
	// step can't replace on its own.
	Manual bool `yaml:"manual,omitempty"`

	SeenWhen *string `yaml:"seen_when"`
	ShownAs  *string `yaml:"shown_as"`
	Triage   *string `yaml:"triage"`
	Proposed *string `yaml:"proposed"`
	// How is the code change the rewrite takes, to judge its cost to the code.
	How     *string `yaml:"how"`
	Verdict *string `yaml:"verdict"`
}

const fields = `What each entry's fields hold:

| Field | Holds |
|---|---|
| where | Each place the text is, as file:line (enclosing function). |
| context | The call or field it's in. |
| now | The text, with a format string's verbs kept, and {expr} for a part joined from an expression. |
| args | A format string's arguments, in order. |
| manual | Joined from a literal and an expression, so it's applied by hand. |
| seen_when | The reader's situation when they meet it. Filled in at triage. |
| shown_as | Where it shows (CLI, API, MCP, log), and the text with realistic values. Filled in at triage. |
| triage | fine, fix, not-user-facing or needs-context. |
| proposed | The rewrite. |
| how | The code change the rewrite takes: what changes where, which tests match the old text, and any cost to the code. |
| verdict | approve, reword or skip. |
`

func main() {
	out := flag.String("out", "tmp/prose", "the folder to write the review tables into")
	force := flag.Bool("force", false, "replace the kinds' folders, losing what was filled in")
	flag.Parse()
	if err := run(*out, *force); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(out string, force bool) error {
	c := &collector{fset: token.NewFileSet(), byKey: map[string]*entry{}, kinds: map[string][]*entry{}}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (d.Name() == "testdata" || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			return c.file(path)
		})
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(out, 0o755); err != nil { //nolint:gosec // G301: review tables, read by people
		return err
	}
	for _, kind := range []string{"errors", "descriptions", "help", "other"} {
		dir := filepath.Join(out, kind)
		if _, err := os.Stat(dir); err == nil {
			if !force {
				return fmt.Errorf("%s is already there, and may hold triage and rewrites. Pass -force to replace it", dir)
			}
			if err := os.RemoveAll(dir); err != nil {
				return err
			}
		}
		if err := write(dir, c.kinds[kind]); err != nil {
			return err
		}
	}
	return nil
}

// A file holds at most this many entries: a review batch.
const batch = 30

// write writes a kind's entries into dir, one file per area of the code (an
// area split when it has more than a batch), and a README.md indexing them.
func write(dir string, entries []*entry) error {
	if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // G301: review tables, read by people
		return err
	}
	byArea := map[string][]*entry{}
	for _, e := range entries {
		a := area(e.Where[0])
		byArea[a] = append(byArea[a], e)
	}
	// An area too small to review on its own joins its group's misc file.
	for a, es := range byArea {
		if len(es) < minArea {
			delete(byArea, a)
			m := misc(a)
			byArea[m] = append(byArea[m], es...)
		}
	}
	var index strings.Builder
	index.WriteString("# " + filepath.Base(dir) + "\n\n" + fields + "\n| File | Entries | Manual |\n|---|---|---|\n")
	total := 0
	for _, a := range slices.Sorted(maps.Keys(byArea)) {
		es := byArea[a]
		slices.SortStableFunc(es, func(x, y *entry) int { return compareWhere(x.Where[0], y.Where[0]) })
		chunks := (len(es) + batch - 1) / batch
		size := (len(es) + chunks - 1) / chunks
		for i := 0; i < len(es); i += size {
			chunk := es[i:min(i+size, len(es))]
			name := a + ".yml"
			if chunks > 1 {
				name = fmt.Sprintf("%s-%d.yml", a, i/size+1)
			}
			b, err := yaml.MarshalWithOptions(chunk, yaml.UseLiteralStyleIfMultiline(true), yaml.IndentSequence(true))
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(dir, name), append([]byte("# The fields are in README.md.\n\n"), b...), 0o644); err != nil { //nolint:gosec // G306: review tables, read by people
				return err
			}
			manual := 0
			for _, e := range chunk {
				if e.Manual {
					manual++
				}
			}
			fmt.Fprintf(&index, "| [%s](%s) | %d | %d |\n", name, name, len(chunk), manual)
			total += len(chunk)
		}
	}
	fmt.Fprintf(&index, "\n%d entries.\n", total)
	fmt.Fprintf(os.Stderr, "%s: %d entries, %d areas\n", dir, total, len(byArea))
	return os.WriteFile(filepath.Join(dir, "README.md"), []byte(index.String()), 0o644) //nolint:gosec // G306: review tables, read by people
}

// An area with fewer entries than this joins its group's misc file.
const minArea = 5

// misc is the file a small area joins, by its group: the CLI, shared, or the rest.
func misc(area string) string {
	for _, group := range []string{"cli", "shared"} {
		if strings.HasPrefix(area, group+"-") {
			return group + "-misc"
		}
	}
	return "misc"
}

// area is where a place in the code belongs: a cmd/anfra file is one command
// ("cli-publish"), and any other file its package ("command-query").
func area(where string) string {
	file, _, _ := strings.Cut(where, ":")
	dir := filepath.ToSlash(filepath.Dir(file))
	if dir == "cmd/anfra" {
		return "cli-" + strings.TrimSuffix(filepath.Base(file), ".go")
	}
	return strings.ReplaceAll(strings.TrimPrefix(dir, "internal/"), "/", "-")
}

// compareWhere orders places by file, then line.
func compareWhere(a, b string) int {
	af, al := splitWhere(a)
	bf, bl := splitWhere(b)
	return cmp.Or(cmp.Compare(af, bf), cmp.Compare(al, bl))
}

func splitWhere(w string) (string, int) {
	file, rest, _ := strings.Cut(w, ":")
	line, _, _ := strings.Cut(rest, " ")
	n, _ := strconv.Atoi(line)
	return file, n
}

type collector struct {
	fset  *token.FileSet
	byKey map[string]*entry
	kinds map[string][]*entry
	// The string literals already taken by a known context, so "other" skips them.
	taken map[token.Pos]bool
	// funcAt names the function around a position in the file being read.
	funcAt func(token.Pos) string
}

func (c *collector) file(path string) error {
	f, err := parser.ParseFile(c.fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return err
	}
	c.taken = map[token.Pos]bool{}
	c.funcAt = enclosing(f)
	// Known contexts first, then any other sentence-like literal.
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			c.call(n)
		case *ast.CompositeLit:
			c.composite(n)
		case *ast.Field:
			if n.Tag != nil {
				c.taken[n.Tag.Pos()] = true
				tag, _ := strconv.Unquote(n.Tag.Value)
				if doc := reflect.StructTag(tag).Get("doc"); doc != "" {
					c.add("descriptions", "doc tag", doc, nil, false, n.Tag.Pos())
				}
			}
		case *ast.ImportSpec:
			c.taken[n.Path.Pos()] = true
		}
		return true
	})
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			// A literal passed to a function: named by that function.
			fn := c.expr(n.Fun)
			kind := "other"
			if errorBuilder(fn) {
				kind = "errors"
			}
			for _, a := range n.Args {
				c.other(kind, "arg of "+fn, a)
			}
		case ast.Expr:
			c.other("other", "string literal", n)
		}
		return true
	})
	return nil
}

// A message argument's index, by the call it's in.
var messageArg = map[string]struct {
	kind string
	arg  int
}{
	"apperr.New":                {"errors", 1},
	"apperr.NewWith":            {"errors", 1},
	"apperr.Encapsulate":        {"errors", 2},
	"apperr.DefinePublicCode":   {"errors", 3},
	"apperr.DefineInternalCode": {"errors", 3},
	"fmt.Errorf":                {"errors", 0},
	"errors.New":                {"errors", 0},
	"fmt.Sprintf":               {"other", 0},
	"fmt.Fprintf":               {"other", 1},
	"fmt.Printf":                {"other", 0},
}

// other adds e, a string literal or literals joined with +, as kind's text if
// it reads as prose and isn't taken.
func (c *collector) other(kind, ctx string, e ast.Expr) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING || c.taken[e.Pos()] {
			return
		}
	case *ast.BinaryExpr:
		if e.Op != token.ADD || !hasFreeLiteral(e, c.taken) {
			return
		}
	default:
		return
	}
	taken := maps.Clone(c.taken)
	s, literal, ok := c.join(e)
	if !ok || !sentenceLike(s) {
		c.taken = taken // let a part of it be found on its own
		return
	}
	c.add(kind, ctx, s, nil, !literal, e.Pos())
}

// errorBuilder reports whether fn makes an error from the text passed to it.
func errorBuilder(fn string) bool {
	return strings.HasPrefix(fn, "clierr.") || strings.HasPrefix(fn, "apperr.") ||
		fn == "command.InvalidArg" || fn == "flagViolated"
}

// hasFreeLiteral reports whether e has a string literal not yet taken.
func hasFreeLiteral(e ast.Expr, taken map[token.Pos]bool) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING && !taken[lit.Pos()] {
			found = true
		}
		return !found
	})
	return found
}

func (c *collector) call(n *ast.CallExpr) {
	sel, ok := n.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	name := c.expr(sel)
	if m, ok := messageArg[name]; ok && len(n.Args) > m.arg {
		var args []string
		if strings.HasPrefix(name, "fmt.") {
			for _, a := range n.Args[m.arg+1:] {
				args = append(args, c.expr(a))
			}
		}
		if m.kind == "other" {
			// Sprintf builds paths and URLs too: only prose is other text.
			if s, _, ok := c.join(n.Args[m.arg]); !ok || !sentenceLike(s) {
				return
			}
		}
		c.text(m.kind, name, n.Args[m.arg], args)
		return
	}
	// A flag's usage is its last argument: (Bool|String|…)(Var)?(P)?.
	if isFlagDef(sel.Sel.Name) && len(n.Args) >= 2 {
		// The name comes first, or after the variable a …Var takes.
		nameArg := n.Args[0]
		if strings.Contains(sel.Sel.Name, "Var") {
			nameArg = n.Args[1]
		}
		ctx := "flag " + c.expr(nameArg) + " usage"
		if lit, ok := nameArg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			c.taken[lit.Pos()] = true
			name, _ := strconv.Unquote(lit.Value)
			ctx = "flag --" + name + " usage"
		}
		c.text("help", ctx, n.Args[len(n.Args)-1], nil)
	}
}

func isFlagDef(name string) bool {
	name = strings.TrimSuffix(strings.TrimSuffix(name, "P"), "Var")
	switch name {
	case "Bool", "String", "Int", "Int64", "Uint", "Float64", "Duration", "Count",
		"StringSlice", "StringArray", "IntSlice", "StringToString":
		return true
	}
	return false
}

func (c *collector) composite(n *ast.CompositeLit) {
	typ := n.Type
	if ix, ok := typ.(*ast.IndexListExpr); ok {
		typ = ix.X
	}
	if ix, ok := typ.(*ast.IndexExpr); ok {
		typ = ix.X
	}
	var kind, of string
	switch c.expr(typ) {
	case "cobra.Command":
		kind, of = "help", "cobra.Command"
	case "command.Def", "Def":
		kind, of = "descriptions", "command.Def"
	default:
		return
	}
	for _, el := range n.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key := c.expr(kv.Key)
		if key == "Short" || key == "Long" || key == "Example" {
			c.text(kind, of+"."+key, kv.Value, nil)
		}
	}
}

// text adds e's text, if it has a string literal in it.
func (c *collector) text(kind, ctx string, e ast.Expr, args []string) {
	s, literal, ok := c.join(e)
	if ok {
		c.add(kind, ctx, s, args, !literal, e.Pos())
	}
}

// join is e's text: its string literals, joined, with {expr} for any other
// part. literal reports whether it's all literals; ok, whether any is.
func (c *collector) join(e ast.Expr) (s string, literal, ok bool) {
	switch e := e.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			c.taken[e.Pos()] = true
			s, err := strconv.Unquote(e.Value)
			return s, true, err == nil
		}
	case *ast.ParenExpr:
		return c.join(e.X)
	case *ast.BinaryExpr:
		if e.Op == token.ADD {
			l, lLit, lOK := c.join(e.X)
			r, rLit, rOK := c.join(e.Y)
			return l + r, lLit && rLit, lOK || rOK
		}
	}
	return "{" + c.expr(e) + "}", false, false
}

func (c *collector) add(kind, ctx, s string, args []string, manual bool, pos token.Pos) {
	p := c.fset.Position(pos)
	where := fmt.Sprintf("%s:%d", p.Filename, p.Line)
	if fn := c.funcAt(pos); fn != "" {
		where += " (" + fn + ")"
	}
	key := kind + "\x00" + s
	if e, ok := c.byKey[key]; ok {
		e.Where = append(e.Where, where)
		if !strings.Contains(e.Context, ctx) {
			e.Context += ", " + ctx
		}
		e.Manual = e.Manual || manual
		return
	}
	e := &entry{Where: []string{where}, Context: ctx, Now: s, Args: args, Manual: manual}
	c.byKey[key] = e
	c.kinds[kind] = append(c.kinds[kind], e)
}

func (c *collector) expr(e ast.Expr) string {
	var b strings.Builder
	_ = printer.Fprint(&b, c.fset, e)
	return b.String()
}

// sentenceLike reports whether s reads as prose: at least four words, with
// letters, and not a path, a URL or code.
func sentenceLike(s string) bool {
	if len(strings.Fields(s)) < 4 || !strings.ContainsFunc(s, unicode.IsLetter) {
		return false
	}
	for _, code := range []string{"{{", "}}", "://", "SELECT ", "select "} {
		if strings.Contains(s, code) {
			return false
		}
	}
	return true
}

// enclosing gives, for a position in f, the function declared around it.
func enclosing(f *ast.File) func(token.Pos) string {
	type span struct {
		from, to token.Pos
		name     string
	}
	var spans []span
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			name := fd.Name.Name
			if fd.Recv != nil && len(fd.Recv.List) > 0 {
				var b strings.Builder
				_ = printer.Fprint(&b, token.NewFileSet(), fd.Recv.List[0].Type)
				name = strings.TrimPrefix(b.String(), "*") + "." + name
			}
			spans = append(spans, span{fd.Pos(), fd.End(), name})
		}
	}
	return func(p token.Pos) string {
		for _, s := range spans {
			if s.from <= p && p < s.to {
				return s.name
			}
		}
		return ""
	}
}
