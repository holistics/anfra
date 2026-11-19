package telemetry

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/holistics/anfra/internal/repoenv"
	caller "github.com/holistics/anfra/shared/telemetry"
)

// Destination is where this process's traces go, and what set it.
type Destination struct {
	// URL is the endpoint; "" when nothing is exported.
	URL string
	// From names what set it, as something a person can unset: an OTEL_*
	// variable, "in .env" when the repo's file set it (internal/repoenv).
	From string
	// variable is the OTEL_* variable that set the endpoint, and fileLine
	// where in the repo's .env it is set, "line 3": "" when the environment
	// set it.
	variable, fileLine string
}

// Current is where traces would go for a process starting now: the OTEL_*
// variables, the repo's .env among them, or nowhere. OTEL_SDK_DISABLED=true is
// nowhere. A running process asks Installed instead.
func Current() Destination {
	d, _ := resolve()
	return d
}

// resolve is Current, for Setup.
func resolve() (Destination, error) {
	if os.Getenv("OTEL_SDK_DISABLED") == "true" {
		return Destination{From: repoenv.Describe("OTEL_SDK_DISABLED")}, nil
	}
	for _, name := range []string{"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "OTEL_EXPORTER_OTLP_ENDPOINT"} {
		if v := os.Getenv(name); v != "" {
			line, _ := repoenv.Origin(name)
			return Destination{URL: v, From: repoenv.Describe(name), variable: name, fileLine: line}, nil
		}
	}
	return Destination{}, nil
}

// Redacted is the URL to show a person, in status, a hint or a log: its
// password and query values replaced with xxxxx, since a backend may take its
// key there.
func (d Destination) Redacted() string {
	u, err := url.Parse(d.URL)
	if err != nil {
		return d.URL
	}
	if u.RawQuery != "" {
		q := u.Query()
		for k := range q {
			q[k] = []string{"xxxxx"}
		}
		u.RawQuery = q.Encode()
	}
	return u.Redacted()
}

// Disabled reports whether OTEL_SDK_DISABLED turned telemetry off.
func (d Destination) Disabled() bool { return d.URL == "" && d.From != "" }

// installed is where Setup sent this process's traces, as Current was then.
var installed Destination

// Installed is where this process's traces go, as Setup decided when it
// started: for a server, which keeps the endpoint it started with.
func Installed() Destination { return installed }

// Answering reports whether something listens at the destination's host and
// port, within timeout: a stopped telemetry stack doesn't.
func (d Destination) Answering(timeout time.Duration) bool {
	u, err := url.Parse(d.URL)
	if err != nil || u.Host == "" {
		return false
	}
	port := u.Port()
	switch {
	case port != "":
	case u.Scheme == "https":
		port = "443"
	default:
		port = "80"
	}
	c, err := net.DialTimeout("tcp", net.JoinHostPort(u.Hostname(), port), timeout)
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}

// ProtocolWarning says why OTEL_EXPORTER_OTLP_PROTOCOL can't be honoured, when
// it asks for gRPC: anfra exports over HTTP only. "" when there is nothing to
// say.
func ProtocolWarning() string {
	for _, name := range []string{"OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "OTEL_EXPORTER_OTLP_PROTOCOL"} {
		if v := os.Getenv(name); v != "" {
			if v == "grpc" {
				return fmt.Sprintf("%s=grpc is set, but anfra sends traces over HTTP only (http/protobuf), so it sends them over HTTP. "+
					"Point the endpoint at your collector's HTTP port (usually 4318), or run a collector that forwards to your gRPC backend.", name)
			}
			return ""
		}
	}
	return ""
}

// Warning is something about where traces go that the user should know.
type Warning struct {
	Code    string
	Message string
}

// GuardWarnings are what the CLI warns of on every command, since the repo's
// .env can turn data on, and agents write the repo without asking: an agent
// told by a doc or a query result to edit the file could send the user's data
// to someone else's endpoint (docs/designs/telemetry.md, setting storages).
//
//   - data_by_default: data is on because nothing set it, and traces leave
//     this machine. Setting it, to 0 or 1, is the user's choice, and stops
//     the warning.
//   - data_from_env_file: .env turns data on, and traces leave this machine.
//   - telemetry_unencrypted: traces go over plain http:// to a machine named
//     on the internet. A name without a dot, such as the telemetry stack's
//     collector on its Docker network (stack.CollectorInNetwork), or a
//     private address, is the user's own network.
func GuardWarnings(dest Destination, data caller.Resolution) []Warning {
	if dest.URL == "" {
		return nil
	}
	u, err := url.Parse(dest.URL)
	if err != nil || isLoopback(u.Hostname()) {
		return nil
	}
	var warnings []Warning
	if data.Data.Value && data.Data.From == "" {
		warnings = append(warnings, Warning{Code: "data_by_default", Message: fmt.Sprintf(
			"Traces record your data, such as filter values and SQL, and go to %s, which is not on this machine (set by %s). "+
				"Set %s=0 to keep your data out of them, or %s=1 to send it and stop this warning.",
			u.Host, dest.From, caller.DataVar, caller.DataVar)})
	}
	if _, inFile := repoenv.Origin(data.Data.From); inFile && data.Data.Value {
		warnings = append(warnings, Warning{Code: "data_from_env_file", Message: fmt.Sprintf(
			"The repo's %s sets %s=1, so your data goes to %s, which is not on this machine. "+
				"If you didn't set it, remove it from %s, and check %s there.",
			repoenv.FileName, caller.DataVar, u.Host, repoenv.FileName, dest.From)})
	}
	if u.Scheme == "http" && !isLocalNetwork(u.Hostname()) {
		warnings = append(warnings, Warning{Code: "telemetry_unencrypted", Message: fmt.Sprintf(
			"Traces go to %s over http://, unencrypted, to another machine. Use https:// unless the network is your own (set by %s).",
			u.Host, dest.From)})
	}
	return warnings
}

// StatusHints are what status says to check when traces leave this machine,
// whatever set what: status, not every command, since a hosted backend is a
// setup the docs recommend (docs/designs/telemetry.md, setting storages).
//
//   - The repo's .env set the endpoint: an agent can edit the file to send
//     traces elsewhere, and even the defaults send the repo's id and each
//     step's name and timing.
//   - The user's data is on, set in the environment or the file.
func (d Destination) StatusHints(data caller.Resolution) []string {
	if d.URL == "" {
		return nil
	}
	u, err := url.Parse(d.URL)
	if err != nil || isLoopback(u.Hostname()) {
		return nil
	}
	var hints []string
	if d.fileLine != "" {
		hints = append(hints, fmt.Sprintf("Traces go to %s, which is not on this machine, because the repo's %s sets %s on %s. "+
			"If you didn't set it there, remove that line.", u.Host, repoenv.FileName, d.variable, d.fileLine))
	}
	if data.Data.Value {
		hints = append(hints, fmt.Sprintf("Traces record your data, turned on by %s, and go to %s, which is not on this machine. "+
			"Unless you run that endpoint yourself, set it to 0.", repoenv.Describe(data.Data.From), u.Host))
	}
	return hints
}

// isLoopback reports whether host is this machine: localhost, or a loopback
// address.
func isLoopback(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// isLocalNetwork reports whether host is on the user's own network: a name
// without a dot, which only local names resolve (a Docker network's, a LAN's),
// or a private or link-local address.
func isLocalNetwork(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		// A number is an address some resolvers read, 3627732462 or
		// 0xd83ad5ae, not a name.
		if _, err := strconv.ParseUint(host, 0, 32); err == nil {
			return false
		}
		return !strings.Contains(host, ".") && !strings.Contains(host, ":")
	}
	return ip.IsPrivate() || ip.IsLinkLocalUnicast()
}
