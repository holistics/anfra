package stack

import (
	"bytes"
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// The compose file is YAML, with each collector file inline as it is, and its
// $ doubled for compose.
func TestTheComposeFileHoldsTheCollectorFiles(t *testing.T) {
	var c struct {
		Name    string `yaml:"name"`
		Configs map[string]struct {
			Content string `yaml:"content"`
		} `yaml:"configs"`
		Networks map[string]struct {
			Name string `yaml:"name"`
		} `yaml:"networks"`
	}
	name := Name("shop-1a2b3c4d")
	if err := yaml.Unmarshal(Compose("test", name, DefaultPorts), &c); err != nil {
		t.Fatal(err)
	}
	if c.Name != name || c.Networks[name].Name != name {
		t.Errorf("project %q, network %+v", c.Name, c.Networks)
	}
	for name, want := range map[string]string{
		"collector-config": collectorConfig, "collector-clickhouse": collectorClickHouse, "collector-debug": collectorDebug,
	} {
		got := strings.ReplaceAll(c.Configs[name].Content, "$$", "$")
		if strings.TrimSpace(got) != strings.TrimSpace(want) {
			t.Errorf("%s differs from its file:\n%s", name, got)
		}
	}
	if got := c.Configs["clickhouse-no-system-logs"].Content; strings.TrimSpace(got) != strings.TrimSpace(clickhouseNoSystemLogs) {
		t.Errorf("ClickHouse's config differs from its file:\n%s", got)
	}
	if !strings.Contains(c.Configs["collector-clickhouse"].Content, "$${env:CLICKHOUSE_TTL:-0}") {
		t.Error("the collector's own variables are not escaped from compose")
	}
}

// A repo's stack is named for its id, in the letters Docker takes.
func TestName(t *testing.T) {
	for id, want := range map[string]string{
		"shop-1a2b3c4d":       "anfra-telemetry-shop-1a2b3c4d",
		"My Shop.v2-1a2b3c4d": "anfra-telemetry-my-shop-v2-1a2b3c4d",
	} {
		if got := Name(id); got != want {
			t.Errorf("Name(%q) = %q, want %q", id, got, want)
		}
	}
}

// Write creates the file, rewrites its own for another version keeping its
// ports, leaves the same one alone, and never touches a compose.yaml it
// didn't write.
func TestWrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "telemetry")
	name := Name("shop-1a2b3c4d")
	var first Ports
	step := func(version string, want Change) {
		t.Helper()
		w, err := Write(dir, Spec{Version: version, Name: name})
		if err != nil || w.Change != want || w.Path != filepath.Join(dir, FileName) {
			t.Fatalf("Write(%s) = %+v, %v; want %s", version, w, err, want)
		}
		if first == (Ports{}) {
			first = w.Ports
		} else if w.Ports != first {
			t.Fatalf("Write(%s) moved the ports from %+v to %+v", version, first, w.Ports)
		}
		if b, _ := os.ReadFile(w.Path); !bytes.Equal(b, Compose(version, name, first)) {
			t.Fatalf("Write(%s) wrote another file", version)
		}
		if p, ok := PortsIn(w.Path); !ok || p != first {
			t.Fatalf("PortsIn = %+v, %v; want %+v", p, ok, first)
		}
	}
	step("1.0.0", Created)
	step("1.0.0", Unchanged)
	step("1.1.0", Updated)

	other := t.TempDir()
	theirs := []byte("services: {}\n")
	if err := os.WriteFile(filepath.Join(other, FileName), theirs, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(other, Spec{Version: "1.0.0", Name: name}); !errors.Is(err, ErrNotOurs) {
		t.Errorf("someone's compose.yaml: %v, want ErrNotOurs", err)
	}
	if b, _ := os.ReadFile(filepath.Join(other, FileName)); !bytes.Equal(b, theirs) {
		t.Error("someone's compose.yaml was changed")
	}
}

// A new stack takes the usual ports when free, and skips those another repo's
// stack holds, or something listens on.
func TestPickPorts(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port
	taken := DefaultPorts.List()
	p := PickPorts(append(taken, busy))
	got := p.List()
	for i, n := range got {
		if slices.Contains(taken, n) || n == busy {
			t.Errorf("picked %d, which is taken: %+v", n, p)
		}
		if slices.Contains(got[i+1:], n) {
			t.Errorf("picked %d twice: %+v", n, p)
		}
	}
}
