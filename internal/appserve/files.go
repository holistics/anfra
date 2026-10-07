package appserve

import (
	"html"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Entry is one node of the Data App tree: a Data App definition, or a folder of them.
type Entry struct {
	Kind     string  `json:"kind"`
	Path     string  `json:"path"`
	Label    string  `json:"label,omitempty"`
	Name     string  `json:"name,omitempty"`
	Children []Entry `json:"children,omitempty"`
}

var titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

// labelOf is a definition's <title>, or its file name.
func labelOf(file string) string {
	raw, _ := os.ReadFile(file) //nolint:gosec // G304: a file Catalog found under apps/
	if m := titleRe.FindSubmatch(raw); m != nil {
		if title := strings.Join(strings.Fields(string(m[1])), " "); title != "" {
			return html.UnescapeString(title)
		}
	}
	return filepath.Base(file)
}

func scan(root, rel string) []Entry {
	dirents, _ := os.ReadDir(filepath.Join(root, rel))
	var folders, apps []Entry
	for _, d := range dirents {
		if strings.HasPrefix(d.Name(), ".") {
			continue
		}
		childRel := d.Name()
		if rel != "" {
			childRel = rel + "/" + d.Name()
		}
		switch {
		case d.IsDir():
			// A folder holding no Data Apps, at any depth, is left out of the tree.
			if children := scan(root, childRel); len(children) > 0 {
				folders = append(folders, Entry{Kind: "folder", Path: childRel, Name: d.Name(), Children: children})
			}
		case d.Type().IsRegular() && strings.HasSuffix(strings.ToLower(d.Name()), ".html"):
			apps = append(apps, Entry{Kind: "app", Path: childRel, Label: labelOf(filepath.Join(root, filepath.FromSlash(childRel)))})
		}
	}
	// Folders first, then Data Apps, each alphabetical (ReadDir sorts), as a file tree reads.
	return append(append([]Entry{}, folders...), apps...)
}

// Catalog is every Data App definition under the repo's apps/, as a tree. Paths are relative to
// apps/, with their .html.
func Catalog(repoDir string) []Entry {
	entries := scan(filepath.Join(repoDir, "apps"), "")
	if entries == nil {
		return []Entry{}
	}
	return entries
}

// appFile is the file a path under apps/ names, or "" when it names none: outside apps/, a
// dot-file or under a dot-directory, or not a regular file.
func appFile(repoDir, rel string) string {
	// Resolved, so a repo reached through a symlink (macOS's /var, say) still matches its files.
	apps, err := filepath.EvalSymlinks(filepath.Join(repoDir, "apps"))
	if err != nil {
		return ""
	}
	if apps, err = filepath.Abs(apps); err != nil {
		return ""
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || strings.HasPrefix(part, ".") {
			return ""
		}
	}
	file, err := filepath.EvalSymlinks(filepath.Join(apps, filepath.FromSlash(rel)))
	if err != nil || !strings.HasPrefix(file, apps+string(os.PathSeparator)) {
		return ""
	}
	// Confined above: under apps/, through symlinks too, no dot-entries.
	if info, err := os.Stat(file); err != nil || !info.Mode().IsRegular() { //nolint:gosec // G703: confined to apps/
		return ""
	}
	return file
}

// file serves a file under apps/: a Data App definition, as written, or what it refers to (an
// image, a script). Never run as the API's origin: a page opened from here is sandboxed, so author
// code reaches the API only through a host's bridge.
func (s *Server) file(w http.ResponseWriter, r *http.Request, rel string) {
	file := appFile(s.opts.RepoDir, rel)
	if file == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Security-Policy", "sandbox allow-scripts")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, file)
}
