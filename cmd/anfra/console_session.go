package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/holistics/anfra/internal/app"
	"github.com/holistics/anfra/internal/cliui"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/shared/apperr"
)

// consoleSession is what a console holds from one entry to the next
// (docs/designs/console.md#lines): its mode, the command that receives plain
// text, and the flags set for every command.
type consoleSession struct {
	// mode is the text mode's command and its flags, as typed after console:
	// ["query", "--lang", "sql"]; nil in command mode.
	mode      []string
	modePath  string // the mode's command, as named: "query", "query compile"
	modeWords bool   // the mode's text is a list of words, not one string
	lang      string // the mode's --lang in effect, deciding how an entry ends; "" without one

	// flags are the session's flags, in the order set: added to every
	// command that takes one and wasn't given it.
	flags []sessionFlag

	styles cliui.TerminalStyles // what it says is styled with: none off a terminal
}

// sessionFlag is a flag set for the session: its long name, and its value,
// "" for a boolean's true.
type sessionFlag struct {
	name, value string
}

// consoleAct is what an entry asks of the console.
type consoleAct int

const (
	actRun   consoleAct = iota // run an anfra command
	actLeave                   // end the session
	actHelp                    // list the commands
	actSay                     // say something: why it can't run, or what changed
)

// dispatch is what an entry asks: an anfra command's args, to leave, the
// commands listed (\?), or something said. In command mode a line is a
// command, a \ before it taken too; in a text mode, a line is the mode's
// text, and a command needs its \. A \ line of flags sets them for the
// session; console <command> (or c) switches the mode, console alone back to
// commands.
func (s *consoleSession) dispatch(entry string) (args []string, act consoleAct, msg string) {
	slashed := strings.HasPrefix(entry, `\`)
	if s.mode != nil && !slashed {
		return s.textArgs(entry)
	}
	rest := strings.TrimSpace(strings.TrimPrefix(entry, `\`))
	switch rest {
	case "q", "exit", "quit":
		return nil, actLeave, ""
	case "?", "":
		return nil, actHelp, ""
	}
	if slashed && strings.HasPrefix(rest, "-") {
		return nil, actSay, s.setFlags(rest)
	}
	args, err := splitArgs(rest)
	if err != nil {
		return nil, actSay, err.Error()
	}
	if args[0] == "console" || args[0] == "c" {
		return nil, actSay, s.switchMode(args[1:])
	}
	return args, actRun, ""
}

// textArgs is the mode's command run on text: the text whole for a string
// argument, its words for a list, after -- so text starting with - isn't
// taken for a flag; an AQL or SQL entry without what ended it.
func (s *consoleSession) textArgs(text string) ([]string, consoleAct, string) {
	switch s.lang {
	case "aql":
		text = strings.TrimSuffix(strings.TrimSpace(text), ";;")
	case "sql":
		text = strings.TrimSuffix(strings.TrimSpace(text), ";")
	}
	args := append(slices.Clone(s.mode), "--")
	if !s.modeWords {
		return append(args, text), actRun, ""
	}
	words, err := splitArgs(text)
	if err != nil {
		return nil, actSay, err.Error()
	}
	return append(args, words...), actRun, ""
}

// switchMode makes modeArgs, a command and its flags, the mode: its text
// typed directly from now on; none, back to command mode. A command that
// takes no text can't be one.
func (s *consoleSession) switchMode(modeArgs []string) string {
	if len(modeArgs) == 0 {
		if s.mode == nil {
			return "Already at commands. console <command> types a command's text directly."
		}
		*s = consoleSession{flags: s.flags, styles: s.styles}
		return s.styles.Muted.Render("Back to commands.")
	}
	root := newRootCmd()
	c, rest, err := root.Find(modeArgs)
	if err != nil || c == root {
		return fmt.Sprintf(`%s isn't a command, and \? lists them. To query a dataset named %[1]s, \-d %[1]s sets it for every command.`, modeArgs[0])
	}
	path := strings.TrimPrefix(c.CommandPath(), root.Name()+" ")
	words, ok := modeArg(c)
	if !ok {
		return fmt.Sprintf("%s takes no text to type. console <command> works with %s.", path, strings.Join(modeCommands(root), ", "))
	}
	if err := c.ParseFlags(rest); err != nil {
		return err.Error()
	}
	if len(c.Flags().Args()) > 0 {
		return fmt.Sprintf("A mode is a command and its flags, since %s takes its text from what you type.", path)
	}
	s.mode, s.modePath, s.modeWords = modeArgs, path, words
	s.refreshLang()
	return s.styles.Muted.Render(s.modeHint()) // as the console's intro is
}

// refreshLang works out the mode's --lang in effect: the mode's own, else the
// session's, else the command's default; "" when its command has none.
func (s *consoleSession) refreshLang() {
	s.lang = ""
	if s.mode == nil {
		return
	}
	c, rest, err := newRootCmd().Find(s.mode)
	if err != nil || c.ParseFlags(rest) != nil {
		return
	}
	f := c.Flags().Lookup("lang")
	if f == nil {
		return
	}
	s.lang = f.Value.String()
	if v, set := s.flag("lang"); set && !f.Changed {
		s.lang = v
	}
}

// modeHint is the mode's intro: how to type in it, where help is, and how to
// leave it.
func (s *consoleSession) modeHint() string {
	return s.howToType() + ` \? helps, \c goes back.`
}

// howToType says how an entry runs in the mode.
func (s *consoleSession) howToType() string {
	switch s.lang {
	case "aql":
		return "Type AQL, ending it with ;; to run it."
	case "sql":
		return "Type SQL, ending it with ; to run it."
	}
	return "Type " + s.modePath + "'s text, and press Enter to run it."
}

// modeArg reports whether c takes text a mode can give it: a core command's
// positional argument, its input (a query, search terms, globs), and whether
// as words (a list) rather than whole (a string). A CLI-only command's
// positional is a folder or an address, not text to type.
func modeArg(c *cobra.Command) (words, ok bool) {
	op, found := app.Find(c.Annotations[opAnnotation])
	if !found {
		return false, false
	}
	for _, a := range op.Args() {
		if a.Positional {
			return a.Type == command.ArgStringArray, true
		}
	}
	return false, false
}

// modeCommands are the commands a console can take text for, as named:
// query, query compile, …, search.
func modeCommands(root *cobra.Command) []string {
	var names []string
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if _, ok := modeArg(c); ok && !c.Hidden {
			names = append(names, strings.TrimPrefix(c.CommandPath(), root.Name()+" "))
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
	return names
}

// ends reports whether text, typed in the session's mode, is a whole entry,
// for Enter to run it rather than start a line: AQL ends with ;;, SQL with
// ;, outside a string or a comment; a \ line, any other mode's text, and an
// empty entry end on Enter.
func (s *consoleSession) ends(text string) bool {
	t := strings.TrimSpace(text)
	if s.mode == nil || t == "" || strings.HasPrefix(t, `\`) {
		return true
	}
	switch s.lang {
	case "aql":
		return terminated(t, ";;", "//")
	case "sql":
		return terminated(t, ";", "--")
	}
	return true
}

// terminated reports whether text ends with end, outside a string ('…', "…"
// or `…`, a backslash escaping) and a comment, from comment to the line's end.
func terminated(text, end, comment string) bool {
	var quote rune
	inComment := false
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch {
		case inComment:
			inComment = c != '\n'
		case quote != 0:
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
		case strings.HasPrefix(string(runes[i:]), comment):
			inComment = true
		case c == '\'' || c == '"' || c == '`':
			quote = c
		}
	}
	return quote == 0 && !inComment && strings.HasSuffix(text, end)
}

// flag is the session's value of the flag name, and whether it's set.
func (s *consoleSession) flag(name string) (string, bool) {
	for _, f := range s.flags {
		if f.name == name {
			return f.value, true
		}
	}
	return "", false
}

// alternativeFlags are flags only one of which a command takes: setting one
// for the session unsets the others, so \--yaml after \--json switches the
// format rather than leaving every command refusing both.
var alternativeFlags = [][]string{{"json", "yaml"}}

// notSessionFlags change what a command does rather than tune it, so a
// session can't set them.
var notSessionFlags = map[string]bool{"help": true, "version": true, "schema": true}

// setFlags sets the flags in line for the session, as `\--dataset sales
// -d x --json` gives them: a long name or a one-letter one, a value after it
// or after =, none for a boolean; an empty value unsets one. `\--` alone lists
// them. A flag no command takes, or a short name meaning different flags in
// different commands, is refused, and nothing is set.
func (s *consoleSession) setFlags(line string) string {
	tokens, err := splitArgs(line)
	if err != nil {
		return err.Error()
	}
	if len(tokens) == 1 && tokens[0] == "--" {
		return s.listFlags()
	}
	flags, shorts := flagCatalog()
	modeCmd := s.modeCommand() // in a text mode, its command: a flag must be one it takes
	next := slices.Clone(s.flags)
	mode := slices.Clone(s.mode) // a flag the mode gives is the mode's to change
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		name, value, hasValue := strings.Cut(strings.TrimLeft(tok, "-"), "=")
		name = strings.ReplaceAll(name, "_", "-")
		switch {
		case !strings.HasPrefix(tok, "-") || name == "":
			return fmt.Sprintf("%s isn't a flag. A line of flags sets them for every command, as \\--dataset sales does.", tok)
		case !strings.HasPrefix(tok, "--") && modeCmd != nil:
			f := modeCmd.Flags().ShorthandLookup(name)
			if f == nil {
				return fmt.Sprintf("%s doesn't take -%s.", s.modePath, name)
			}
			name = f.Name
		case !strings.HasPrefix(tok, "--"):
			longs := shorts[name]
			if len(longs) > 1 {
				return fmt.Sprintf("-%s is %s, so give the long name.", name, strings.Join(longs, " and "))
			}
			if len(longs) == 0 {
				return fmt.Sprintf("No command takes -%s.", name)
			}
			name = strings.TrimPrefix(strings.Fields(longs[0])[0], "--")
		}
		if modeCmd != nil && (modeCmd.Flags().Lookup(name) == nil || notSessionFlags[name]) {
			return fmt.Sprintf("%s doesn't take --%s. See \\? for its flags.", s.modePath, name)
		}
		f := flags[name]
		if f == nil || notSessionFlags[name] {
			return fmt.Sprintf("No command takes --%s for a session to set.", name)
		}
		isBool := f.Value.Type() == "bool"
		switch {
		case hasValue:
		case isBool:
			value = "true"
		case i+1 < len(tokens):
			i++
			value = tokens[i]
		default:
			return fmt.Sprintf("--%s needs a value. Set it with \\--%s <value>, or unset it with \\--%s=.", name, name, name)
		}
		if modeCmd != nil && modeCmd.Flags().Lookup(name).Changed {
			// The mode gives it, and an entry's own flags win: so it's the
			// mode's that changes, as c <command> with it would.
			mode = withoutFlag(mode, modeCmd.Flags().Lookup(name))
			if value != "" && (!isBool || value != "false") {
				mode = append(mode, "--"+name+"="+value)
			}
			continue
		}
		next = slices.DeleteFunc(next, func(f sessionFlag) bool { return f.name == name })
		if value != "" && (!isBool || value != "false") {
			for _, alts := range alternativeFlags {
				if slices.Contains(alts, name) {
					next = slices.DeleteFunc(next, func(f sessionFlag) bool { return slices.Contains(alts, f.name) })
				}
			}
		}
		if isBool && value == "true" {
			value = ""
		} else if value == "" || isBool && value == "false" {
			continue
		}
		next = append(next, sessionFlag{name: name, value: value})
	}
	s.flags = next
	if !slices.Equal(mode, s.mode) {
		return s.switchMode(mode)
	}
	lang := s.lang
	s.refreshLang()
	if s.lang != lang { // a mode now read another way says so, as switching to it does
		return s.styles.Muted.Render(s.modeHint())
	}
	return ""
}

// listFlags is \--'s answer: the session's flags, a line each, their names
// blue as keys are, one the mode's command doesn't take said so; then, in a
// text mode, the mode's other flags, which \? describes.
func (s *consoleSession) listFlags() string {
	modeCmd := s.modeCommand()
	var b strings.Builder
	if modeCmd != nil {
		var set []string
		modeCmd.Flags().Visit(func(f *pflag.Flag) {
			set = append(set, "\n  "+s.styles.Key.Render("--"+f.Name)+"   "+f.Value.String())
		})
		if len(set) > 0 {
			b.WriteString("Set by the mode:" + strings.Join(set, "") + "\n")
		}
	}
	if len(s.flags) == 0 {
		b.WriteString(`No flags set in this session. \--<flag> <value> sets one for every command.`)
	} else {
		width := 0
		for _, f := range s.flags {
			width = max(width, len(f.name))
		}
		b.WriteString("Set in this session:")
		for _, f := range s.flags {
			b.WriteString("\n  " + s.styles.Key.Render("--"+f.name) + strings.Repeat(" ", width-len(f.name)+3) + f.value)
			if modeCmd != nil && modeCmd.Flags().Lookup(f.name) == nil {
				b.WriteString(s.styles.Muted.Render(" (not for " + s.modePath + ")"))
			}
		}
	}
	if modeCmd == nil {
		return strings.TrimRight(b.String(), " ")
	}
	var others []string
	modeCmd.Flags().VisitAll(func(f *pflag.Flag) {
		if _, set := s.flag(f.Name); !set && !f.Changed && !f.Hidden && !notSessionFlags[f.Name] {
			others = append(others, s.styles.Key.Render("--"+f.Name))
		}
	})
	if len(others) > 0 {
		b.WriteString("\nOthers available: " + strings.Join(others, " "))
	}
	return b.String()
}

// withoutFlag is args without f, in any form typed: --name value,
// --name=value, -n value, -n=value, -nvalue; a boolean without a value.
func withoutFlag(args []string, f *pflag.Flag) []string {
	takesValue := f.Value.Type() != "bool"
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		long, short := "--"+f.Name, ""
		if f.Shorthand != "" {
			short = "-" + f.Shorthand
		}
		switch {
		case a == long || short != "" && a == short:
			if takesValue {
				i++ // its value
			}
		case strings.HasPrefix(a, long+"="), short != "" && strings.HasPrefix(a, short) && !strings.HasPrefix(a, "--"):
		default:
			out = append(out, a)
		}
	}
	return out
}

// modeCommand is the mode's command, its flags parsed, the root's merged in;
// nil in command mode.
func (s *consoleSession) modeCommand() *cobra.Command {
	if s.mode == nil {
		return nil
	}
	c, rest, err := newRootCmd().Find(s.mode)
	if err != nil || c.ParseFlags(rest) != nil {
		return nil
	}
	return c
}

// flagCatalog is every flag anfra's commands take, by long name, and each
// one-letter name's long names, as "--dataset (query, …)", for a session's
// flags: the root's output flags included, hidden ones (former names) not.
func flagCatalog() (map[string]*pflag.Flag, map[string][]string) {
	root := newRootCmd()
	flags := map[string]*pflag.Flag{}
	used := map[string]map[string][]string{} // short -> long -> commands
	add := func(path string, f *pflag.Flag) {
		if f.Hidden {
			return
		}
		if _, ok := flags[f.Name]; !ok {
			flags[f.Name] = f
		}
		if f.Shorthand != "" {
			if used[f.Shorthand] == nil {
				used[f.Shorthand] = map[string][]string{}
			}
			used[f.Shorthand][f.Name] = append(used[f.Shorthand][f.Name], path)
		}
	}
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) { add("every command", f) })
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		path := strings.TrimPrefix(c.CommandPath(), root.Name()+" ")
		c.LocalNonPersistentFlags().VisitAll(func(f *pflag.Flag) { add(path, f) })
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	for _, c := range root.Commands() {
		walk(c)
	}
	shorts := map[string][]string{}
	for short, longs := range used {
		for _, long := range slices.Sorted(func(yield func(string) bool) {
			for l := range longs {
				if !yield(l) {
					return
				}
			}
		}) {
			shorts[short] = append(shorts[short], fmt.Sprintf("--%s for %s", long, strings.Join(longs[long], ", ")))
		}
	}
	return flags, shorts
}

// withFlags is args with the session's flags its command takes and wasn't
// given, before any -- (the mode's text after it): the entry's own winning.
// A format flag isn't added to a command that writes an artifact, which
// refuses it.
func (s *consoleSession) withFlags(args []string) []string {
	if len(s.flags) == 0 {
		return args
	}
	root := newRootCmd()
	c, rest, err := root.Find(args)
	if err != nil || c.ParseFlags(rest) != nil {
		return args // the command says what's wrong when it runs
	}
	var added []string
	for _, f := range s.flags {
		pf := c.Flags().Lookup(f.name)
		if pf == nil || pf.Changed {
			continue
		}
		if c.Annotations[outputKind] == outputArtifact && (f.name == "json" || f.name == "yaml" || f.name == "jq") {
			continue
		}
		if f.value == "" {
			added = append(added, "--"+f.name)
		} else {
			added = append(added, "--"+f.name+"="+f.value)
		}
	}
	at := slices.Index(args, "--")
	if at < 0 {
		at = len(args)
	}
	return slices.Concat(args[:at], added, args[at:])
}

// prompt is the console's prompt: the mode, which decides what a line is,
// "query sql> "; "> " in command mode. Where, and with what flags, is the
// line under it (context).
func (s *consoleSession) prompt() string {
	if s.mode == nil {
		return "> "
	}
	p := s.modePath
	if s.lang != "" && s.lang != "aql" {
		p += " " + s.lang
	}
	return p + "> "
}

// targetFlags name what a command runs against, a dataset or a data source:
// their values are picked out wherever the session's flags are shown, the one
// thing in them a person checks before running a query.
var targetFlags = map[string]bool{"dataset": true, "data-source": true}

// contextPart is a piece of where the session is, and whether it's a target's
// name, picked out.
type contextPart struct {
	text   string
	target bool
}

// contextParts is where the session is: the folder's name, the mode when
// withMode, and the flags in effect as typed, the mode's and the session's,
// short names where they have them: "acme · -d sales --json".
func (s *consoleSession) contextParts(folder string, withMode bool) []contextPart {
	parts := []contextPart{{text: folder}}
	if withMode && s.mode != nil {
		parts = append(parts, contextPart{text: " · " + s.modePath + " " + s.textName()})
	}
	var flags []contextPart
	add := func(text string, target bool) {
		if len(flags) > 0 {
			flags = append(flags, contextPart{text: " "})
		}
		flags = append(flags, contextPart{text: text, target: target})
	}
	modeFlags := s.modeFlags()
	c := s.modeCommand()
	for i := 0; i < len(modeFlags); i++ {
		tok := modeFlags[i]
		name, value, inline := strings.Cut(strings.TrimLeft(tok, "-"), "=")
		var f *pflag.Flag
		if c != nil && strings.HasPrefix(tok, "--") {
			f = c.Flags().Lookup(name)
		} else if c != nil && len(name) > 0 {
			f = c.Flags().ShorthandLookup(name[:1])
			if !inline && len(name) > 1 {
				value, inline = name[1:], true // -dsales
			}
		}
		switch {
		case f == nil || f.Value.Type() == "bool":
			add(tok, false)
		case inline:
			add(strings.TrimSuffix(tok, value), false) // the value follows with no space
			flags = append(flags, contextPart{text: value, target: targetFlags[f.Name]})
		case i+1 < len(modeFlags):
			add(tok, false)
			i++
			add(modeFlags[i], targetFlags[f.Name])
		default:
			add(tok, false)
		}
	}
	catalog, _ := flagCatalog()
	for _, f := range s.flags {
		name := "--" + f.name
		if pf := catalog[f.name]; pf != nil && pf.Shorthand != "" {
			name = "-" + pf.Shorthand
		}
		add(name, false)
		if f.value != "" {
			flags = append(flags, contextPart{text: " "}, contextPart{text: f.value, target: targetFlags[f.name]})
		}
	}
	if len(flags) > 0 {
		parts = append(parts, contextPart{text: " · "})
		parts = append(parts, flags...)
	}
	return parts
}

// context is where the session is, as plain text: "acme · -d sales --json".
func (s *consoleSession) context(folder string) string {
	var b strings.Builder
	for _, p := range s.contextParts(folder, false) {
		b.WriteString(p.text)
	}
	return b.String()
}

// styledFlags is the flags in effect, as typed, for \?: names blue as keys
// are, values plain, a target's name bold; "" when none.
func (s *consoleSession) styledFlags() string {
	parts := s.contextParts("", false)
	if len(parts) < 3 {
		return ""
	}
	var b strings.Builder
	for _, p := range parts[2:] { // past the folder and its " · "
		switch {
		case p.target:
			b.WriteString(s.styles.Heading.Render(p.text))
		case strings.HasPrefix(p.text, "-"):
			b.WriteString(s.styles.Key.Render(p.text))
		default:
			b.WriteString(p.text)
		}
	}
	return b.String()
}

// styledContext is where the session is, styled for the line under the
// frame, or \?'s first line (withMode): faint, a target's name bold, picked
// out, as a heading's weight would.
func (s *consoleSession) styledContext(folder string, withMode bool) string {
	var b strings.Builder
	for _, p := range s.contextParts(folder, withMode) {
		if p.target {
			b.WriteString(s.styles.Heading.Render(p.text))
		} else {
			b.WriteString(s.styles.Muted.Render(p.text))
		}
	}
	return b.String()
}

// modeFlags are the flags the mode was entered with, as typed, but its
// --lang, which the prompt shows: c query -d a gives [-d a].
func (s *consoleSession) modeFlags() []string {
	c := s.modeCommand()
	if c == nil {
		return nil
	}
	tokens := s.mode[len(strings.Fields(s.modePath)):]
	if lang := c.Flags().Lookup("lang"); lang != nil {
		tokens = withoutFlag(tokens, lang)
	}
	return tokens
}

// flagProblem is the first problem the mode's command finds with a flag, the
// mode's flags and the session's together, as its own check before running
// does (inputChecks): the flag's long name, "--dataset"; as it's set, its
// short name where it has one, "-d"; the violation's code, "required" for one
// missing; and its message. All "" when none.
func (s *consoleSession) flagProblem() (long, short, code, msg string) {
	if s.mode == nil {
		return "", "", "", ""
	}
	args := s.withFlags(append(slices.Clone(s.mode), "--", "x")) // x: the text, any
	root := newRootCmd()
	c, rest, err := root.Find(args)
	if err != nil || c.ParseFlags(rest) != nil || inputChecks[c] == nil {
		return "", "", "", ""
	}
	vs, ok := apperr.DetailsOf(inputChecks[c](c.Flags().Args()), apperr.ValidationFailed)
	if !ok {
		return "", "", "", ""
	}
	op, _ := app.Find(c.Annotations[opAnnotation])
	for _, v := range vs.Violations {
		for _, a := range op.Args() {
			if a.Name == v.Field && !a.Positional {
				long = "--" + a.Flag()
				short = long
				if a.Shorthand != "" {
					short = "-" + a.Shorthand
				}
				return long, short, v.Code, v.Message
			}
		}
	}
	return "", "", "", ""
}
