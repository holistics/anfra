package sidecar

import "strings"

// A spawned sidecar inherits anfra's environment, so whatever it needs from the
// machine (HOME, PATH, the dynamic loader's variables, a driver's credentials)
// reaches it without anfra having to know the list. What it does not inherit
// are the names anfra owns:
//
//   - anfra's own (ANFRA_*): its settings, and secrets such as its tokens. A
//     sidecar that reads an ANFRA_ name (anfra-node's ANFRA_PORT) gets it from
//     its supervisor, for that sidecar, or not at all.
//   - Per-process settings (stripped): each process's own, so one set for anfra
//     must not also land on a sidecar, as two pprof servers on one address or
//     three processes' traces under one service name. A runner sets one for a
//     sidecar with that sidecar's prefix (ProcSpec.EnvPrefix).
//
// stripped lists only names that collide this way. Anything else is passed on.
var stripped = map[string]bool{
	"PPROF_ADDR":               true, // canal-query's pprof server
	"OTEL_SERVICE_NAME":        true, // each process names its own traces
	"OTEL_RESOURCE_ATTRIBUTES": true,
	"GOMEMLIMIT":               true, // the Go runtime's, per process
	"GOMAXPROCS":               true,
	"GOGC":                     true,
	"NODE_OPTIONS":             true, // Node's, for anfra-node's runtime
}

// childEnv is the environment a sidecar is spawned with, from environ (anfra's,
// as os.Environ gives it): what it inherits, then each <prefix><NAME>=value as
// NAME=value, then fixed, the values its supervisor sets, which nothing
// overrides. A name appears once, with the last value given for it.
func childEnv(environ []string, prefix string, fixed []string) []string {
	var inherited, forwarded []string
	for _, kv := range environ {
		name, value, _ := strings.Cut(kv, "=")
		switch {
		case prefix != "" && strings.HasPrefix(name, prefix):
			if name := strings.TrimPrefix(name, prefix); name != "" {
				forwarded = append(forwarded, name+"="+value)
			}
		case strings.HasPrefix(name, "ANFRA_"), stripped[name]:
		default:
			inherited = append(inherited, kv)
		}
	}
	return lastWins(append(append(inherited, forwarded...), fixed...))
}

// lastWins keeps each name's last entry, in the place of its first.
func lastWins(env []string) []string {
	last := map[string]string{}
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		last[name] = kv
	}
	out := make([]string, 0, len(last))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if v, ok := last[name]; ok {
			out = append(out, v)
			delete(last, name)
		}
	}
	return out
}
