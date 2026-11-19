// Package stack is anfra's telemetry stack, as a compose file: an
// OpenTelemetry collector, and ClickHouse to keep what it receives
// (docs/designs/telemetry.md). The file is embedded, so it always matches the
// anfra that writes it, and nothing is downloaded. anfra only writes it: the
// user runs it with Docker, which starts, stops and deletes it.
package stack

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"
)

var (
	//go:embed compose.yaml.tmpl
	composeTmpl string
	//go:embed otel-collector/config.yaml
	collectorConfig string
	//go:embed otel-collector/clickhouse.yaml
	collectorClickHouse string
	//go:embed otel-collector/debug.yaml
	collectorDebug string
	//go:embed clickhouse/no-system-logs.xml
	clickhouseNoSystemLogs string
)

// Prefix opens every stack's name: its compose project and its Docker network,
// which another container joins to reach the collector at CollectorInNetwork.
// Each repo has its own stack (Name).
const (
	Prefix             = "anfra-telemetry"
	CollectorInNetwork = "http://otel-collector:4318"
)

// Name is the stack of the repo with id repoID: its compose project and its
// network, anfra-telemetry-<repo id>, in the letters Docker takes for a name
// (lowercase letters, digits, - and _).
func Name(repoID string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(repoID) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return Prefix + "-" + b.String()
}

// The stack's ClickHouse user, password and database.
const (
	User     = "anfra"
	Password = "anfra"
	Database = "anfra_traces"
)

// DataSource is the name the usage page gives the data source on the stack's
// ClickHouse, to query the traces with SQL. `anfra publish` never offers it to
// an org: it is this machine's.
const DataSource = "anfra_traces"

// DataSourceYAML is DataSource as an entry under data_sources: in
// data_sources.yml, on the stack's ports p. It speaks ClickHouse's native protocol, as
// canal-query's driver does. Its database holds otel_traces, so a query names
// the table alone.
func DataSourceYAML(p Ports) string {
	return fmt.Sprintf(`  %s:
    type: clickhouse
    connection:
      host: localhost
      port: %d
      dbname: %s
      user: %s
      password: %s
`, DataSource, p.ClickHouse, Database, User, Password)
}

// Ports are the stack's ports on this machine, written in its compose file.
type Ports struct {
	OTLPHTTP, OTLPGRPC, ClickHouseHTTP, ClickHouse int
}

// DefaultPorts are each port's usual number: a repo's stack takes them when
// they are free.
var DefaultPorts = Ports{OTLPHTTP: 4318, OTLPGRPC: 4317, ClickHouseHTTP: 8123, ClickHouse: 9000}

// List is the ports, in the order of their fields.
func (p Ports) List() []int { return []int{p.OTLPHTTP, p.OTLPGRPC, p.ClickHouseHTTP, p.ClickHouse} }

// Endpoint is the collector's OTLP/HTTP endpoint on this machine.
func (p Ports) Endpoint() string { return fmt.Sprintf("http://localhost:%d", p.OTLPHTTP) }

// portLine finds each port in a compose file Write wrote: the published side
// of a mapping to the container's port.
var portLine = regexp.MustCompile(`"127\.0\.0\.1:(\d+):(4318|4317|8123|9000)"`)

// PortsIn are the ports the compose file at path publishes, and whether it is
// a file anfra wrote that publishes all four.
func PortsIn(path string) (Ports, bool) {
	b, err := os.ReadFile(path) //nolint:gosec // G304: a compose file under anfra's folder
	if err != nil || !bytes.HasPrefix(b, []byte(header)) {
		return Ports{}, false
	}
	return portsOf(b)
}

func portsOf(b []byte) (Ports, bool) {
	var p Ports
	for _, m := range portLine.FindAllSubmatch(b, -1) {
		n, _ := strconv.Atoi(string(m[1]))
		switch string(m[2]) {
		case "4318":
			p.OTLPHTTP = n
		case "4317":
			p.OTLPGRPC = n
		case "8123":
			p.ClickHouseHTTP = n
		case "9000":
			p.ClickHouse = n
		}
	}
	return p, !slices.Contains(p.List(), 0)
}

// PickPorts are ports for a new stack: each the usual number when it is free
// and not in taken (another repo's stack, stopped now), else the next one up
// that is.
func PickPorts(taken []int) Ports {
	used := slices.Clone(taken)
	pick := func(from int) int {
		for p := from; p < from+1000; p++ {
			if !slices.Contains(used, p) && free(p) {
				used = append(used, p)
				return p
			}
		}
		return from // nothing free near it: compose will say which port is taken
	}
	d := DefaultPorts
	return Ports{OTLPHTTP: pick(d.OTLPHTTP), OTLPGRPC: pick(d.OTLPGRPC), ClickHouseHTTP: pick(d.ClickHouseHTTP), ClickHouse: pick(d.ClickHouse)}
}

// free reports whether port is free on this machine's loopback.
func free(port int) bool {
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = ln.Close()
	return true
}

// FileName is the compose file's name, in the folder Write writes it to.
const FileName = "compose.yaml"

// header opens every compose file anfra writes: how Write tells its own file,
// which it rewrites, from someone else's, which it leaves alone.
const header = "# anfra's telemetry stack"

// Change is what Write did to the compose file.
type Change string

const (
	Created   Change = "created"
	Updated   Change = "updated"
	Unchanged Change = "unchanged"
)

// ErrNotOurs is a compose.yaml in the folder that anfra didn't write.
var ErrNotOurs = errors.New("a compose.yaml anfra didn't write is there")

// Spec is the stack Write writes: for anfra Version, named Name, on ports
// picked around Taken the first time.
type Spec struct {
	Version, Name string
	// Taken are ports a new stack avoids: other repos' stacks', which may be
	// stopped now.
	Taken []int
}

// Written is what Write did: the file, its ports, and the change.
type Written struct {
	Path   string
	Ports  Ports
	Change Change
}

// Write writes the compose file for s into dir, creating dir: Created, Updated
// when anfra wrote the file there before (for another version, say), or
// Unchanged when it is the same. A file anfra wrote keeps its ports, so the
// endpoint stays the same across upgrades; a new one gets free ports. A
// compose.yaml that anfra didn't write is left alone: ErrNotOurs.
func Write(dir string, s Spec) (Written, error) {
	w := Written{Path: filepath.Join(dir, FileName), Change: Created}
	have, err := os.ReadFile(w.Path) //nolint:gosec // G304: the repo's folder under anfra's
	switch {
	case errors.Is(err, fs.ErrNotExist):
		w.Ports = PickPorts(s.Taken)
	case err != nil:
		return w, err
	case !bytes.HasPrefix(have, []byte(header)):
		return w, ErrNotOurs
	default:
		w.Change = Updated
		var ok bool
		if w.Ports, ok = portsOf(have); !ok {
			w.Ports = PickPorts(s.Taken)
		}
	}
	want := Compose(s.Version, s.Name, w.Ports)
	if bytes.Equal(have, want) {
		w.Change = Unchanged
		return w, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return w, err
	}
	tmp := w.Path + ".tmp"
	if err := os.WriteFile(tmp, want, 0o600); err != nil {
		return w, err
	}
	if err := os.Rename(tmp, w.Path); err != nil {
		_ = os.Remove(tmp)
		return w, err
	}
	return w, nil
}

// Compose is the compose file of the stack named name on ports p, for anfra
// version: one file, the collector's configuration inline.
func Compose(version, name string, p Ports) []byte {
	t := template.Must(template.New("compose").Funcs(template.FuncMap{"indent": indent}).Parse(composeTmpl))
	var b bytes.Buffer
	if err := t.Execute(&b, map[string]any{
		"Version": version, "Name": name, "Ports": p, "Endpoint": p.Endpoint(),
		"Config": collectorConfig, "ClickHouse": collectorClickHouse, "Debug": collectorDebug,
		"NoSystemLogs": clickhouseNoSystemLogs,
	}); err != nil {
		panic(err) // the template is embedded: only a broken build fails
	}
	return b.Bytes()
}

// indent is a file inline under a compose key: each line indented six spaces,
// and each $ doubled, which compose reads as one.
func indent(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = "      " + strings.ReplaceAll(l, "$", "$$")
		}
	}
	return strings.Join(lines, "\n")
}
