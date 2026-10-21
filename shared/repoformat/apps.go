package repoformat

import (
	"bytes"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// CheckOrigin reports whether s may be added to a Data App's content policy:
// https://<host>[:<port>] and nothing after it, written as a browser writes an
// origin (lowercase, no default port), so it cannot widen a directive (a scheme,
// a wildcard) or end one (a space, a ";"). The SDK's checkOrigin is the same
// rule, in the browser.
func CheckOrigin(s string) error {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.Opaque != "" || u.User != nil ||
		u.Path != "" || u.RawQuery != "" || u.Fragment != "" || strings.HasSuffix(s, "?") || strings.HasSuffix(s, "#") ||
		u.Port() == "443" || strings.ContainsAny(u.Host, "* ;") || u.Host != strings.ToLower(u.Host) || !ascii(u.Host) ||
		s != "https://"+u.Host {
		return fmt.Errorf("%q is not an origin: write https://<host>[:<port>], with no path, wildcard or trailing slash", s)
	}
	return nil
}

func ascii(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

// AppIssue is something in a Data App definition that will not work in a host:
// where it is, and what to do about it.
type AppIssue struct {
	Line    int
	Message string
}

func (i AppIssue) String() string { return fmt.Sprintf("line %d: %s", i.Line, i.Message) }

var (
	cssURLRe = regexp.MustCompile(`url\(\s*(?:"([^"]*)"|'([^']*)'|([^)'"\s]*))\s*\)`)
	schemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
)

// CheckApp is what in a Data App definition will not work in a host, in
// document order:
//
//   - A reference to a local file (a relative or root-relative src, href,
//     srcset, poster or CSS url()): a Data App is one HTML file, and a host
//     serves nothing beside it.
//   - A script or stylesheet from another server without an integrity hash:
//     the library runs beside the reader's query results, so a change to it at
//     its CDN should stop it, not run it.
//
// It never fails: a definition it cannot read has no issues it can name.
func CheckApp(content []byte) []AppIssue {
	var issues []AppIssue
	add := func(line int, format string, args ...any) {
		issues = append(issues, AppIssue{Line: line, Message: fmt.Sprintf(format, args...)})
	}
	local := func(line int, ref string) {
		add(line, "%q is a file beside the app, which no host serves: put it in the HTML (inline <script>, <style>, SVG or a data: URL), or load it from a CDN by its https:// URL", ref)
	}
	cssRefs := func(line int, css string) {
		for _, m := range cssURLRe.FindAllStringSubmatchIndex(css, -1) {
			ref := ""
			for g := 2; g < len(m); g += 2 {
				if m[g] >= 0 {
					ref += css[m[g]:m[g+1]]
				}
			}
			if isLocal(ref) {
				local(line+strings.Count(css[:m[0]], "\n"), ref)
			}
		}
	}

	z := html.NewTokenizer(bytes.NewReader(content))
	line, inStyle := 1, false
	for {
		tt := z.Next()
		if tt == html.ErrorToken { // the end: it reads from memory
			return issues
		}
		raw := z.Raw()
		at := line
		line += bytes.Count(raw, []byte("\n"))
		switch tt {
		case html.TextToken:
			if inStyle {
				cssRefs(at, string(raw))
			}
		case html.EndTagToken:
			if name, _ := z.TagName(); string(name) == "style" {
				inStyle = false
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, more := z.TagName()
			tag := string(name)
			attrs := map[string]string{}
			for more {
				var k, v []byte
				k, v, more = z.TagAttr()
				attrs[string(k)] = string(v)
			}
			if tag == "style" && tt == html.StartTagToken {
				inStyle = true
			}
			if style, ok := attrs["style"]; ok {
				cssRefs(at, style)
			}
			for _, a := range []string{"src", "href", "poster"} {
				if ref, ok := attrs[a]; ok && isLocal(ref) {
					local(at, ref)
				}
			}
			if srcset, ok := attrs["srcset"]; ok {
				for _, c := range strings.Split(srcset, ",") {
					if f := strings.Fields(c); len(f) > 0 && isLocal(f[0]) {
						local(at, f[0])
					}
				}
			}
			_, hashed := attrs["integrity"]
			switch {
			case tag == "script" && isRemote(attrs["src"]) && !hashed:
				add(at, "the script %s has no integrity hash: add integrity=\"sha384-…\" crossorigin=\"anonymous\"", attrs["src"])
			case tag == "link" && hasToken(attrs["rel"], "stylesheet") && isRemote(attrs["href"]) && !hashed:
				add(at, "the stylesheet %s has no integrity hash: add integrity=\"sha384-…\" crossorigin=\"anonymous\"", attrs["href"])
			}
		}
	}
}

// isLocal is whether ref names a file beside the app: no scheme, not
// protocol-relative, not a fragment or empty.
func isLocal(ref string) bool {
	ref = strings.TrimSpace(ref)
	return ref != "" && !strings.HasPrefix(ref, "#") && !strings.HasPrefix(ref, "//") && !schemeRe.MatchString(ref)
}

// isRemote is whether ref is loaded from another server.
func isRemote(ref string) bool {
	ref = strings.ToLower(strings.TrimSpace(ref))
	return strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "//")
}

func hasToken(list, token string) bool {
	for _, t := range strings.Fields(strings.ToLower(list)) {
		if t == token {
			return true
		}
	}
	return false
}
