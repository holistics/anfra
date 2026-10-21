package publish

import (
	"slices"
	"strings"
	"testing"
)

// localSources is a data_sources.yml with warehouse and dev, which the org lacks,
// and events, which it has as another type. Only warehouse has settings.
const localSources = `data_sources:
  warehouse:
    type: postgresql
    connection:
      host: db.example.com
      port: 5432
      password: hunter2
    settings:
      require_ssl: true
  dev:
    type: postgresql
    connection:
      host: 127.0.0.1
  events:
    type: bigquery
    connection:
      project: p
`

// Each data source the org lacks is asked about in turn, checked with the
// connection the person agreed to send, and created when the server reaches it;
// one it cannot reach is created only when chosen. One the org has is never
// changed, and a different type is said.
func TestPublishOffersMissingDataSources(t *testing.T) {
	s, url := newServer(t)
	s.sources["events"] = "snowflake"
	s.unreachable = map[string]string{"dev": "connection refused: 127.0.0.1:5432"}
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "a", ".anfra/data_sources.yml": localSources})

	ask := &script{t: t, answers: []string{"yes", "skip creating this data source", "yes"}} // dev: create, then skip once unreachable; warehouse: create
	_, errOut, err := run(t, dir, Options{SetOrg: url}, ask.ask)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Data source dev (postgresql) is not on the org yet. Create it? Its connection, credentials included, is sent to the server.",
		"Create dev anyway?",
		"Data source warehouse (postgresql) is not on the org yet. Create it? Its connection, credentials included, is sent to the server.",
	}
	if !slices.Equal(ask.asked, want) {
		t.Errorf("asked %q\nwant  %q", ask.asked, want)
	}
	if !strings.Contains(errOut, "✗ Data source dev (postgresql) is not reachable from the org's server:\n    connection refused: 127.0.0.1:5432\n") {
		t.Errorf("the failed check is not said: %q", errOut)
	}
	if _, ok := s.created["dev"]; ok {
		t.Error("dev was created though skipped")
	}
	if got := s.created["warehouse"]; got["password"] != "hunter2" || got["port"] == nil || s.sources["warehouse"] != "postgresql" {
		t.Errorf("warehouse created as %v (%s)", got, s.sources["warehouse"])
	}
	if s.settings["warehouse"]["require_ssl"] != true || s.settings["dev"] != nil {
		t.Errorf("created with settings %v, want warehouse's alone", s.settings)
	}
	if strings.Contains(errOut, "settings are not sent") {
		t.Errorf("settings said unsent to a server that takes them: %q", errOut)
	}
	if _, ok := s.created["events"]; ok || s.sources["events"] != "snowflake" || !strings.Contains(errOut, "events is bigquery here and snowflake on the org") {
		t.Errorf("events: created %v, now %s; said %q", s.created["events"], s.sources["events"], errOut)
	}
	if !strings.Contains(errOut, "Queries on the models using dev fail until the org has them") || len(s.deployed) != 1 {
		t.Errorf("said %q; deployed %v", errOut, s.deployed)
	}
}

// A server from before data sources had settings gets one without them, saying
// so: the data source is still created.
func TestPublishToAServerWithoutSettings(t *testing.T) {
	s, url := newServer(t)
	s.sources["events"] = "snowflake"
	s.noSettings = true
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "a", ".anfra/data_sources.yml": localSources})

	ask := &script{t: t, answers: []string{"no", "yes"}} // dev: not now; warehouse: create
	_, errOut, err := run(t, dir, Options{SetOrg: url}, ask.ask)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.created["warehouse"]; !ok || s.settings["warehouse"] != nil {
		t.Errorf("warehouse: created %v, settings %v; want created without them", s.created["warehouse"], s.settings["warehouse"])
	}
	if strings.Count(errOut, "The settings of data source warehouse (postgresql) are not sent") != 1 {
		t.Errorf("the unsent settings are not said once: %q", errOut)
	}
}

// A DuckDB data source is never offered, nor sent with --create-data-sources: it
// is a file on this machine. It is warned about, and counted among those the
// org lacks.
func TestPublishWarnsAboutDuckDB(t *testing.T) {
	for _, create := range []bool{false, true} {
		s, url := newServer(t)
		dir := t.TempDir()
		write(t, dir, map[string]string{"a.aml": "a", ".anfra/data_sources.yml": `data_sources:
  local:
    type: duckdb
    connection:
      path: data/shop.duckdb
`})
		ask := &script{t: t} // asks nothing
		_, errOut, err := run(t, dir, Options{SetOrg: url, CreateDataSources: create}, ask.ask)
		if err != nil {
			t.Fatal(err)
		}
		if len(ask.asked) != 0 || s.tested != 0 || len(s.created) != 0 {
			t.Errorf("create=%v: asked %q, tested %d, created %v; want none of it", create, ask.asked, s.tested, s.created)
		}
		if !strings.Contains(errOut, "! Data source local (duckdb) uses a local file") ||
			!strings.Contains(errOut, "Queries on the models using local fail until the org has them") {
			t.Errorf("create=%v: said %q", create, errOut)
		}
	}
}

// One the server cannot reach is created when the person chooses to.
func TestPublishCreatesAnUnreachableDataSourceWhenChosen(t *testing.T) {
	s, url := newServer(t)
	s.unreachable = map[string]string{"dev": "connection refused"}
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "a", ".anfra/data_sources.yml": localSources})
	ask := &script{t: t, answers: []string{"yes", "create it anyway", "no", "no"}} // dev, events, warehouse
	if _, _, err := run(t, dir, Options{SetOrg: url}, ask.ask); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.created["dev"]; !ok {
		t.Error("dev was not created, though chosen")
	}
	if _, ok := s.created["warehouse"]; ok {
		t.Error("warehouse was created, though declined")
	}
}

// Declined, or with no terminal and no --create-data-sources, nothing is sent, not
// even to check it, and the publish goes ahead. --create-data-sources creates
// what the server reaches without asking, and skips what it does not, since no
// one is there to choose.
func TestPublishCreatesDataSourcesOnlyWhenAgreed(t *testing.T) {
	s, url := newServer(t)
	s.unreachable = map[string]string{"dev": "connection refused"}
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "a", ".anfra/data_sources.yml": localSources})

	declined := &script{t: t, answers: []string{"no", "no", "no"}} // dev, events, warehouse
	if _, _, err := run(t, dir, Options{SetOrg: url}, declined.ask); err != nil {
		t.Fatal(err)
	}
	_, errOut, err := run(t, dir, Options{SetOrg: url}, noTerminal)
	if err != nil || len(s.created) != 0 || s.tested != 0 || !strings.Contains(errOut, "--create-data-sources") {
		t.Fatalf("not agreed: err %v, created %v, %d checks; said %q", err, s.created, s.tested, errOut)
	}
	if len(s.deployed) != 1 {
		t.Error("the publish did not go ahead")
	}
	_, errOut, err = run(t, dir, Options{SetOrg: url, CreateDataSources: true}, noTerminal)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.created["warehouse"]; !ok {
		t.Errorf("--create-data-sources: created %v", s.created)
	}
	if _, ok := s.created["dev"]; ok || !strings.Contains(errOut, "Skipping it: there is no terminal to choose") {
		t.Errorf("an unreachable one without a terminal: created %v; said %q", s.created, errOut)
	}
}

// One who may not list or add data sources publishes all the same, told what to
// ask an admin for.
func TestPublishWithoutDataSourcePermissions(t *testing.T) {
	s, url := newServer(t)
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "a", ".anfra/data_sources.yml": localSources})

	s.forbidList = true
	_, errOut, err := run(t, dir, Options{SetOrg: url, CreateDataSources: true}, never(t))
	if err != nil || !strings.Contains(errOut, "An org admin can check it has dev, events, warehouse") {
		t.Errorf("listing refused: %v, said %q", err, errOut)
	}
	s.forbidList, s.forbidCreate = false, true
	write(t, dir, map[string]string{"a.aml": "a2"})
	_, errOut, err = run(t, dir, Options{SetOrg: url, CreateDataSources: true}, never(t))
	if err != nil || !strings.Contains(errOut, "Only an org admin can add data sources: ask one to add dev, events, warehouse") || len(s.created) != 0 {
		t.Errorf("adding refused: %v, created %v, said %q", err, s.created, errOut)
	}
	if len(s.deployed) != 1 || string(s.blobs[s.deployed[0].OID]) != "a2" {
		t.Error("the publish did not go ahead")
	}
}

// A repo with no data_sources.yml here is offered nothing; one whose file does not
// read is told, and publishes.
func TestPublishWithoutLocalDataSources(t *testing.T) {
	s, url := newServer(t)
	s.forbidList = true // not even asked
	dir := t.TempDir()
	write(t, dir, map[string]string{"a.aml": "a"})
	if _, errOut, err := run(t, dir, Options{SetOrg: url}, never(t)); err != nil || strings.Contains(errOut, "data source") {
		t.Errorf("%v, said %q", err, errOut)
	}
}
