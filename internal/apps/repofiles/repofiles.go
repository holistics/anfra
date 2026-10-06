// Package repofiles reads a Data Folder: its anfra config, its Data Apps, and changes to either.
package repofiles

import (
	"fmt"
	"html"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/holistics/anfra/internal/apps/anfra"
)

func configDir(dataFolder string) string {
	name := os.Getenv("ANFRA_DIR_NAME")
	if name == "" {
		name = ".anfra"
	}
	return filepath.Join(dataFolder, name)
}

// Address is where a Data Source's database listens.
type Address struct {
	Name string
	Host string
	Port int
}

var defaultPorts = map[string]int{"postgresql": 5432, "mysql": 3306, "sqlserver": 1433}

// DataSources reads the network address of each Data Source in the anfra config.
func DataSources(dataFolder string) ([]Address, error) {
	file := filepath.Join(configDir(dataFolder), "data_sources.yml")
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, anfra.Startupf("No Data Source config at %s. Is the Data Folder right?", file)
	}
	var parsed struct {
		DataSources map[string]struct {
			Type       string `yaml:"type"`
			Connection struct {
				Host string `yaml:"host"`
				Port any    `yaml:"port"`
			} `yaml:"connection"`
		} `yaml:"data_sources"`
	}
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return nil, anfra.Startupf("Couldn't parse %s: %v", file, err)
	}
	var out []Address
	for name, source := range parsed.DataSources {
		// A source without a network address (an embedded database) has nothing to reach.
		if source.Connection.Host == "" {
			continue
		}
		port := defaultPorts[source.Type]
		switch p := source.Connection.Port.(type) {
		case int:
			port = p
		case string:
			if n, err := strconv.Atoi(p); err == nil {
				port = n
			}
		}
		if port > 0 {
			out = append(out, Address{Name: name, Host: source.Connection.Host, Port: port})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// CheckDataSources refuses to start when a Data Source's database can't be reached, rather than
// serve empty Data Apps.
func CheckDataSources(dataFolder string) error {
	sources, err := DataSources(dataFolder)
	if err != nil {
		return err
	}
	var down []string
	for _, s := range sources {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(s.Host, strconv.Itoa(s.Port)), 2*time.Second)
		if err != nil {
			down = append(down, fmt.Sprintf("  - %s (%s:%d)", s.Name, s.Host, s.Port))
			continue
		}
		_ = conn.Close()
	}
	if len(down) > 0 {
		return anfra.Startupf("Can't reach the database for these Data Sources:\n%s\nStart the database, or fix %s.",
			strings.Join(down, "\n"), filepath.Join(configDir(dataFolder), "data_sources.yml"))
	}
	return nil
}

// CheckContextSources says how to fix a Data Folder anfra can't read datasets from.
func CheckContextSources(dataFolder string) error {
	file := filepath.Join(configDir(dataFolder), "context_sources.yml")
	if _, err := os.Stat(file); err != nil {
		return anfra.Startupf("No %s. anfra needs it to read the Data Folder's datasets; add:\n\nsources:\n  - name: aml\n    type: aml\n    path: ..\n", file)
	}
	return nil
}

// Entry is one node of the Data App tree: a Data App, or a folder of them.
type Entry struct {
	Kind     string  `json:"kind"`
	Path     string  `json:"path"`
	Label    string  `json:"label,omitempty"`
	Name     string  `json:"name,omitempty"`
	Children []Entry `json:"children,omitempty"`
}

var titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

func labelOf(file string) string {
	raw, _ := os.ReadFile(file)
	if m := titleRe.FindSubmatch(raw); m != nil {
		if title := strings.Join(strings.Fields(string(m[1])), " "); title != "" {
			return html.UnescapeString(title)
		}
	}
	return filepath.Base(file)
}

// ReservedName is the top-level name the demo keeps for its own routes (the Shell's reserved namespace).
const ReservedName = "_anfra"

// reserved is whether a top-level entry of apps/ would put a Data App URL inside the reserved namespace.
func reserved(name string) bool {
	return name == ReservedName || strings.EqualFold(name, ReservedName+".html")
}

// ReservedApps are the entries of apps/ left out of the catalog because their URL would be reserved.
func ReservedApps(dataFolder string) []string {
	var out []string
	dirents, _ := os.ReadDir(filepath.Join(dataFolder, "apps"))
	for _, d := range dirents {
		if reserved(d.Name()) {
			out = append(out, "apps/"+d.Name())
		}
	}
	return out
}

func scan(root, rel string) []Entry {
	dirents, _ := os.ReadDir(filepath.Join(root, rel))
	var folders, apps []Entry
	for _, d := range dirents {
		if strings.HasPrefix(d.Name(), ".") || (rel == "" && reserved(d.Name())) {
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
	// Folders first, then Data Apps, each alphabetical (ReadDir already sorts), as a file tree reads.
	return append(append([]Entry{}, folders...), apps...)
}

// Catalog is every Data App under the Data Folder's apps/, as a tree.
func Catalog(dataFolder string) []Entry {
	entries := scan(filepath.Join(dataFolder, "apps"), "")
	if entries == nil {
		return []Entry{}
	}
	return entries
}

// AppFile is the file for a Data App path, or "" when it isn't one (outside apps/, or not HTML).
func AppFile(dataFolder, appPath string) string {
	apps, _ := filepath.Abs(filepath.Join(dataFolder, "apps"))
	file, _ := filepath.Abs(filepath.Join(apps, filepath.FromSlash(appPath)))
	if !strings.HasPrefix(file, apps+string(os.PathSeparator)) || !strings.HasSuffix(strings.ToLower(file), ".html") {
		return ""
	}
	if info, err := os.Stat(file); err != nil || !info.Mode().IsRegular() {
		return ""
	}
	return file
}

// Change is what a burst of file events amounted to.
type Change struct {
	// Apps are the changed paths under apps/ (Data Apps, or folders of them).
	Apps []string
	// AML is whether any AML file changed.
	AML bool
}

// classify says what a changed path under the Data Folder is, if anything the demo cares about.
func classify(rel string) (app string, aml bool, ok bool) {
	rel = filepath.ToSlash(rel)
	for _, part := range strings.Split(rel, "/") {
		if strings.HasPrefix(part, ".") {
			return "", false, false
		}
	}
	if rel == "apps" || strings.HasPrefix(rel, "apps/") {
		return strings.TrimPrefix(strings.TrimPrefix(rel, "apps"), "/"), false, true
	}
	if strings.HasSuffix(strings.ToLower(rel), ".aml") {
		return "", true, true
	}
	return "", false, false
}

type debouncer struct {
	mu     sync.Mutex
	timer  *time.Timer
	apps   map[string]bool
	aml    bool
	delay  time.Duration
	notify func(Change)
}

func (d *debouncer) add(app string, isApp, aml bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if isApp {
		d.apps[app] = true
	}
	d.aml = d.aml || aml
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.delay, d.flush)
}

func (d *debouncer) flush() {
	d.mu.Lock()
	change := Change{AML: d.aml}
	for app := range d.apps {
		change.Apps = append(change.Apps, app)
	}
	sort.Strings(change.Apps)
	d.apps, d.aml = map[string]bool{}, false
	d.mu.Unlock()
	if len(change.Apps) > 0 || change.AML {
		d.notify(change)
	}
}
