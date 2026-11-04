package pretty

import (
	"bytes"
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/goccy/go-yaml/lexer"
	"github.com/goccy/go-yaml/token"

	"github.com/holistics/anfra/internal/cliui"
)

// Tones are the values ColorYAML and ColorJSON give a tone, by their key's path,
// dot-joined from the top: {"error.code": "bad"}. As a pretty view's tone hint,
// "good", "warn" or "bad".
type Tones map[string]string

// ColorYAML is b, YAML as yaml.v3 writes it, colored by cli-pretty.md's rules
// (docs/designs/cli-pretty.md#color): weight for structure, hue for meaning.
// Every key is bright blue, siblings alike, the indentation showing what opens
// a map or a list; null and punctuation faint; a URL as a link; a value with a tone
// in its hue; any other value plain. Only escape codes are added: removed,
// they leave b. Without styles (not a terminal, NO_COLOR), b as it is.
func ColorYAML(b []byte, styles cliui.TerminalStyles, tones Tones) []byte {
	p := paintOf(styles)
	if p.off() {
		return b
	}
	src := string(b)
	tokens := lexer.Tokenize(src)
	var out strings.Builder
	var rebuilt strings.Builder
	var keys []yamlKey // the keys open at this point, outermost first
	var path string    // the path of the key whose value is on this line
	for i, tk := range tokens {
		rebuilt.WriteString(tk.Origin)
		next := tokenAfter(tokens, i)
		var style paintStyle
		switch {
		case next != nil && next.Type == token.MappingValueType:
			col := tk.Position.Column
			for len(keys) > 0 && keys[len(keys)-1].col >= col {
				keys = keys[:len(keys)-1]
			}
			keys = append(keys, yamlKey{col: col, name: tk.Value})
			path = keyPath(keys)
			style = p.key
		case tk.Type == token.MappingValueType, tk.Type == token.SequenceEntryType,
			tk.Type == token.DocumentHeaderType, tk.Type == token.DocumentEndType,
			tk.Type == token.SequenceStartType, tk.Type == token.SequenceEndType,
			tk.Type == token.MappingStartType, tk.Type == token.MappingEndType,
			tk.Type == token.CollectEntryType:
			style = p.muted
		case tk.Type == token.NullType:
			style = p.muted
		default:
			style = p.value(path, tones, tk.Value)
		}
		out.WriteString(style.wrap(tk.Origin))
		if strings.Contains(trailing(tk.Origin), "\n") {
			path = "" // a value is on its key's line only
		}
	}
	// The lexer leaves out what ends the text, a newline: kept as it is. Text it
	// can't give back as it was is left uncolored.
	rest, ok := strings.CutPrefix(src, rebuilt.String())
	if !ok {
		return b
	}
	out.WriteString(rest)
	colored := []byte(out.String())
	if ansi.Strip(string(colored)) != src {
		return b
	}
	return colored
}

type yamlKey struct {
	col  int
	name string
}

func keyPath(keys []yamlKey) string {
	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = k.name
	}
	return strings.Join(names, ".")
}

// tokenAfter is the token after tokens[i], nil at the end.
func tokenAfter(tokens token.Tokens, i int) *token.Token {
	if i+1 < len(tokens) {
		return tokens[i+1]
	}
	return nil
}

// ColorJSON is b, JSON lines as the CLI writes them (each a compact document),
// colored by ColorYAML's rules: every key bright blue, null and punctuation faint, a
// URL as a link, a value with a tone in its hue. Only escape codes are added;
// without styles, b as it is.
func ColorJSON(b []byte, styles cliui.TerminalStyles, tones Tones) []byte {
	p := paintOf(styles)
	if p.off() {
		return b
	}
	var out bytes.Buffer
	var stack []jsonScope // the objects and arrays open, outermost first
	for i := 0; i < len(b); {
		c := b[i]
		switch c {
		case '"':
			end := stringEnd(b, i)
			text := string(b[i:end])
			j := skipSpace(b, end)
			if j < len(b) && b[j] == ':' && len(stack) > 0 && stack[len(stack)-1].object {
				stack[len(stack)-1].key = unquote(text)
				out.WriteString(p.key.wrap(text))
			} else {
				out.WriteString(p.value(jsonPath(stack), tones, unquote(text)).wrap(text))
			}
			i = end
		case '{', '[':
			stack = append(stack, jsonScope{object: c == '{'})
			out.WriteString(p.muted.wrap(string(c)))
			i++
		case '}', ']':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			out.WriteString(p.muted.wrap(string(c)))
			i++
		case ',', ':':
			out.WriteString(p.muted.wrap(string(c)))
			i++
		case ' ', '\t', '\n', '\r':
			out.WriteByte(c)
			i++
		default: // a number, true, false or null
			end := i
			for end < len(b) && !strings.ContainsRune(",]}: \t\r\n", rune(b[end])) {
				end++
			}
			text := string(b[i:end])
			style := p.value(jsonPath(stack), tones, text)
			if text == "null" {
				style = p.muted
			}
			out.WriteString(style.wrap(text))
			i = end
		}
	}
	if ansi.Strip(out.String()) != string(b) {
		return b
	}
	return out.Bytes()
}

type jsonScope struct {
	object bool
	key    string // an object's key being read
}

// jsonPath is the path of the value being read: the keys of the objects open,
// arrays adding none.
func jsonPath(stack []jsonScope) string {
	var names []string
	for _, s := range stack {
		if s.object {
			names = append(names, s.key)
		}
	}
	return strings.Join(names, ".")
}

// stringEnd is the index after the JSON string starting at b[i].
func stringEnd(b []byte, i int) int {
	for j := i + 1; j < len(b); j++ {
		switch b[j] {
		case '\\':
			j++
		case '"':
			return j + 1
		}
	}
	return len(b)
}

func skipSpace(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r') {
		i++
	}
	return i
}

// unquote is a JSON string's text, near enough to name a key or spot a URL.
func unquote(s string) string {
	return strings.TrimSuffix(strings.TrimPrefix(s, `"`), `"`)
}

func isURL(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://")
}

// paint is the escape codes each style puts around a part: before, after.
type paint struct {
	plain, key, muted, link, good, warn, bad paintStyle
}

type paintStyle [2]string

func paintOf(s cliui.TerminalStyles) paint {
	of := func(st interface{ Render(...string) string }) paintStyle {
		before, after, _ := strings.Cut(st.Render("x"), "x")
		return paintStyle{before, after}
	}
	return paint{
		key: of(s.Key), muted: of(s.Muted), link: of(s.Link),
		good: of(s.Success), warn: of(s.Warning), bad: of(s.Error),
	}
}

func (p paint) off() bool { return p.key == paintStyle{} && p.muted == paintStyle{} }

func (p paint) tone(name string) paintStyle {
	return map[string]paintStyle{"good": p.good, "warn": p.warn, "bad": p.bad}[name]
}

// value is how a value at path is styled: its tone, a link, or plain.
func (p paint) value(path string, tones Tones, text string) paintStyle {
	if tone, ok := tones[path]; ok {
		return p.tone(tone)
	}
	if isURL(text) {
		return p.link
	}
	return p.plain
}

// wrap styles s's text, its spaces and line breaks around it left as they are,
// each line apart, so a style never spans a line break.
func (s paintStyle) wrap(text string) string {
	if s == (paintStyle{}) {
		return text
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		core := strings.TrimSpace(line)
		if core == "" {
			continue
		}
		start := strings.Index(line, core)
		lines[i] = line[:start] + s[0] + core + s[1] + line[start+len(core):]
	}
	return strings.Join(lines, "\n")
}

func trailing(s string) string { return s[len(strings.TrimRight(s, " \t\r\n")):] }
