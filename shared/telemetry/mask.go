package telemetry

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// The maskers of the masked level: each keeps its text's shape and replaces
// what the user wrote with ?.

// dialect is how a warehouse's SQL quotes: what ends a string, and which
// quotes start one.
type dialect struct {
	backslash    bool // a backslash escapes the next character in a string
	doubleString bool // "…" is a string, not an identifier
	triple       bool // '''…''' and """…""" are strings
	dollar       bool // $$…$$ and $tag$…$tag$ are strings
	hashComment  bool // # starts a comment to the end of the line
}

// dialects are the warehouses whose quoting differs from the standard's, by
// db.system.name, which is data_sources.yml's type.
var dialects = map[string]dialect{
	"postgresql": {dollar: true},
	"postgres":   {dollar: true},
	"redshift":   {dollar: true},
	"duckdb":     {dollar: true},
	"motherduck": {dollar: true},
	"snowflake":  {backslash: true, dollar: true},
	"mysql":      {backslash: true, doubleString: true, hashComment: true},
	"mariadb":    {backslash: true, doubleString: true, hashComment: true},
	"bigquery":   {backslash: true, doubleString: true, triple: true, hashComment: true},
	"databricks": {backslash: true, doubleString: true},
	"spark":      {backslash: true, doubleString: true},
	"clickhouse": {backslash: true},
	"athena":     {},
	"presto":     {},
	"trino":      {},
	"sqlserver":  {},
	"mssql":      {},
}

// unknownDialect is the quoting of a warehouse not in dialects: a backslash
// escapes, since reading one that doesn't as one that does masks too much,
// and the other way, too little.
var unknownDialect = dialect{backslash: true, dollar: true}

// MaskSQL is sql with each literal, a string or a number, replaced with ?, and
// its comments left out, as the warehouse system (db.system.name) quotes.
// Identifiers stay, quoted or not, so the query keeps its shape.
func MaskSQL(sql, system string) string {
	d, ok := dialects[system]
	if !ok {
		d = unknownDialect
	}
	var b strings.Builder
	b.Grow(len(sql))
	for i := 0; i < len(sql); {
		c := sql[i]
		switch {
		case c == '-' && strings.HasPrefix(sql[i:], "--"), c == '#' && d.hashComment:
			i = lineEnd(sql, i)
			b.WriteByte(' ')
		case c == '/' && strings.HasPrefix(sql[i:], "/*"):
			if end := strings.Index(sql[i+2:], "*/"); end >= 0 {
				i += 2 + end + 2
			} else {
				i = len(sql)
			}
			b.WriteByte(' ')
		case c == '\'':
			i = stringEnd(sql, i, d)
			b.WriteByte('?')
		case c == '"' && d.doubleString:
			i = stringEnd(sql, i, d)
			b.WriteByte('?')
		case c == '"', c == '`':
			end := quotedEnd(sql, i, c)
			b.WriteString(sql[i:end])
			i = end
		case c == '[' && (system == "sqlserver" || system == "mssql"):
			end := quotedEnd(sql, i, ']')
			b.WriteString(sql[i:end])
			i = end
		case c == '$' && d.dollar:
			if tag, ok := dollarTag(sql[i:]); ok {
				if end := strings.Index(sql[i+len(tag):], tag); end >= 0 {
					i += len(tag) + end + len(tag)
				} else {
					i = len(sql)
				}
				b.WriteByte('?')
				continue
			}
			b.WriteByte(c)
			i++
		case startsNumber(sql, i):
			i = numberEnd(sql, i)
			b.WriteByte('?')
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

// lineEnd is where the line that holds i ends, before its newline.
func lineEnd(s string, i int) int {
	if end := strings.IndexByte(s[i:], '\n'); end >= 0 {
		return i + end
	}
	return len(s)
}

// stringEnd is where the string that starts at i ends, after its closing
// quote; the end of s when it isn't closed.
func stringEnd(s string, i int, d dialect) int {
	q := s[i]
	if d.triple && strings.HasPrefix(s[i:], strings.Repeat(string(q), 3)) {
		closing := strings.Repeat(string(q), 3)
		for j := i + 3; j < len(s); j++ {
			if d.backslash && s[j] == '\\' {
				j++
				continue
			}
			if strings.HasPrefix(s[j:], closing) {
				return j + 3
			}
		}
		return len(s)
	}
	// A raw string (BigQuery's r'…') has no escapes.
	backslash := d.backslash && (i == 0 || (s[i-1] != 'r' && s[i-1] != 'R'))
	// Postgres's E'…' does.
	if i > 0 && (s[i-1] == 'E' || s[i-1] == 'e') && (i == 1 || !isIdent(s[i-2])) {
		backslash = true
	}
	for j := i + 1; j < len(s); j++ {
		switch {
		case backslash && s[j] == '\\':
			j++
		case s[j] == q && j+1 < len(s) && s[j+1] == q:
			j++ // a doubled quote is one quote
		case s[j] == q:
			return j + 1
		}
	}
	return len(s)
}

// quotedEnd is where the identifier quoted from i ends, after closing.
func quotedEnd(s string, i int, closing byte) int {
	for j := i + 1; j < len(s); j++ {
		if s[j] == closing {
			if j+1 < len(s) && s[j+1] == closing {
				j++
				continue
			}
			return j + 1
		}
	}
	return len(s)
}

// dollarTag is the $tag$ that starts s, and whether one does: $$, or $name$.
// A parameter ($1) is no tag.
func dollarTag(s string) (string, bool) {
	for j := 1; j < len(s); j++ {
		switch c := s[j]; {
		case c == '$':
			return s[:j+1], true
		case c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (j > 1 && c >= '0' && c <= '9'):
		default:
			return "", false
		}
	}
	return "", false
}

// isIdent reports whether c can be part of a name: a number after one is part
// of it (t1), not a literal.
func isIdent(c byte) bool {
	return c == '_' || c == '$' || c == '@' || c >= 0x80 ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// startsNumber reports whether a number starts at i: a digit, or a point and
// a digit, that isn't part of a name.
func startsNumber(s string, i int) bool {
	if i > 0 && (isIdent(s[i-1]) || s[i-1] == '.') {
		return false
	}
	c := s[i]
	if c >= '0' && c <= '9' {
		return true
	}
	return c == '.' && i+1 < len(s) && s[i+1] >= '0' && s[i+1] <= '9'
}

// numberEnd is where the number that starts at i ends: its digits, point,
// exponent and suffix, 1.5e-3, 0x1F, 10L.
func numberEnd(s string, i int) int {
	j := i
	for j < len(s) {
		c := s[j]
		switch {
		case isIdent(c) || c == '.':
			j++
			if (c == 'e' || c == 'E') && j < len(s) && (s[j] == '+' || s[j] == '-') {
				j++
			}
		default:
			return j
		}
	}
	return j
}

// MaskError is an error message with each quoted string and each number
// replaced with ?, the quotes kept: invalid date '?'. Best effort: a value
// the warehouse doesn't quote, such as a table path, stays.
func MaskError(msg string) string {
	var b strings.Builder
	b.Grow(len(msg))
	prev := rune(0)
	for i := 0; i < len(msg); {
		r, size := utf8.DecodeRuneInString(msg[i:])
		closing, quote := closingQuote(r)
		switch {
		// An apostrophe inside a word (can't, users') is no quote.
		case quote && (r != '\'' || (!unicode.IsLetter(prev) && !unicode.IsDigit(prev))):
			b.WriteRune(r)
			b.WriteByte('?')
			end := strings.IndexRune(msg[i+size:], closing)
			if end < 0 {
				return b.String()
			}
			b.WriteRune(closing)
			i += size + end + utf8.RuneLen(closing)
			prev = closing
			continue
		case unicode.IsDigit(r) && !unicode.IsLetter(prev) && prev != '_':
			j := i
			for j < len(msg) {
				r, size := utf8.DecodeRuneInString(msg[j:])
				if !unicode.IsDigit(r) && !unicode.IsLetter(r) && r != '.' && r != '_' {
					break
				}
				// A sentence's full stop ends the number.
				if r == '.' && (j+size >= len(msg) || !unicode.IsDigit(rune(msg[j+size]))) {
					break
				}
				j += size
			}
			b.WriteByte('?')
			i = j
			prev = '?'
			continue
		}
		b.WriteRune(r)
		i += size
		prev = r
	}
	return b.String()
}

// closingQuote is the quote that closes one opened with r, and whether r opens
// one.
func closingQuote(r rune) (rune, bool) {
	switch r {
	case '\'', '"':
		return r, true
	case '‘':
		return '’', true
	case '“':
		return '”', true
	}
	return 0, false
}
