package query

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/dataperm"
	"github.com/holistics/anfra/internal/datasource"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/querycache"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/internal/sidecar/anfracanal"
	"github.com/holistics/anfra/internal/sidecar/canallake"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/jsonkit"
)

// A hit answers what the miss did: the same header and the same rows, byte for
// byte, for every column type anfra answers. Each case runs once on a warehouse
// (canal-query's DuckDB, in memory) and once from the result it cached.
//
// The answer's header comes from the one stored with the entry, which hides
// what canal-lake changed in it. So each case also reads the entry through
// canal-query directly, and holds canal-lake's own header to the miss's, but
// for the differences known and restored (see readCacheEntry). A new
// difference fails here, to be fixed in canal-lake's write or read.
//
// It needs anfra-canal's binary, or both services', and is skipped without
// them:
//
//	ANFRA_CANAL_BIN=… go test ./internal/command/query -run TestCacheParity
func TestCacheParity(t *testing.T) {
	if os.Getenv(anfracanal.BinEnv) == "" && (os.Getenv(anfracanal.Lake.BinEnv) == "" || os.Getenv(anfracanal.Query.BinEnv) == "") {
		t.Skip("set ANFRA_CANAL_BIN to run against canal-query and canal-lake")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	canal := startCanal(ctx, t)
	qc := newStandIn(ctx, t)
	r := queryRun{cc: command.CommandContext{QueryCache: qc, TenantID: "acme"}, in: QueryRequestParams{Cache: CacheOptions{Duration: "1h"}}}

	// A column of one type, with a NULL row.
	column := func(typ, value string) string {
		return fmt.Sprintf("SELECT * FROM (VALUES ((%s)::%s), (NULL)) t(v)", value, typ)
	}
	cases := []struct {
		name   string
		sql    string
		bounds query.ResultBounds
	}{
		{"tinyint", column("TINYINT", "-128"), query.Unbounded},
		{"smallint", column("SMALLINT", "-32768"), query.Unbounded},
		{"integer", column("INTEGER", "-2147483648"), query.Unbounded},
		{"bigint", column("BIGINT", "-9223372036854775808"), query.Unbounded},
		{"hugeint", column("HUGEINT", "170141183460469231731687303715884105727"), query.Unbounded},
		{"utinyint", column("UTINYINT", "255"), query.Unbounded},
		{"usmallint", column("USMALLINT", "65535"), query.Unbounded},
		{"uinteger", column("UINTEGER", "4294967295"), query.Unbounded},
		{"ubigint", column("UBIGINT", "18446744073709551615"), query.Unbounded},
		{"decimal(18,2)", column("DECIMAL(18,2)", "-1234567890123456.78"), query.Unbounded},
		{"decimal(38,10)", column("DECIMAL(38,10)", "1234567890123456789012345678.0123456789"), query.Unbounded},
		{"float", "SELECT * FROM (VALUES (1.5::FLOAT), ('NaN'::FLOAT), ('inf'::FLOAT), ('-inf'::FLOAT), (NULL)) t(v)", query.Unbounded},
		{"double", "SELECT * FROM (VALUES (0.1::DOUBLE), ('NaN'::DOUBLE), ('inf'::DOUBLE), ('-inf'::DOUBLE), (NULL)) t(v)", query.Unbounded},
		{"varchar", column("VARCHAR", "'héllo, 世界 \"quoted\"\n'"), query.Unbounded},
		{"blob", column("BLOB", `'\xAA\x00\xFF'`), query.Unbounded},
		{"uuid", column("UUID", "'4ac7a9e8-1c3a-4a1b-9e8f-2d5c3b6a7f10'"), query.Unbounded},
		{"enum", "SELECT * FROM (VALUES ('b'::ENUM('a', 'b')), (NULL)) t(v)", query.Unbounded},
		{"boolean", "SELECT * FROM (VALUES (true), (false), (NULL)) t(v)", query.Unbounded},
		{"date", column("DATE", "'2026-11-14'"), query.Unbounded},
		{"time", column("TIME", "'23:59:59.123456'"), query.Unbounded},
		{"interval", column("INTERVAL", "'1 year 2 months 3 days 04:05:06.789'"), query.Unbounded},
		{"timestamp", column("TIMESTAMP", "'2026-11-14 01:02:03.456789'"), query.Unbounded},
		{"timestamp_s", column("TIMESTAMP_S", "'2026-11-14 01:02:03'"), query.Unbounded},
		{"timestamp_ms", column("TIMESTAMP_MS", "'2026-11-14 01:02:03.456'"), query.Unbounded},
		{"timestamp_ns", column("TIMESTAMP_NS", "'2026-11-14 01:02:03.456789123'"), query.Unbounded},
		{"timestamptz", column("TIMESTAMPTZ", "'2026-11-14 01:02:03.456789+07'"), query.Unbounded},
		{"list", column("INTEGER[]", "[1, NULL, 3]"), query.Unbounded},
		{"map", column("MAP(VARCHAR, INTEGER)", "MAP {'a': 1, 'b': NULL}"), query.Unbounded},
		{"struct", column("STRUCT(a INTEGER, b VARCHAR)", "{'a': 1, 'b': 'x'}"), query.Unbounded},
		{"list of structs", column("STRUCT(a INTEGER, b DATE)[]", "[{'a': 1, 'b': '2026-11-14'}, NULL]"), query.Unbounded},
		{"duplicate names", "SELECT 1 AS a, 2 AS a", query.Unbounded},
		{"empty", "SELECT 1 AS a WHERE false", query.Unbounded},
		{"truncated", "SELECT * FROM range(10) t(v)", query.ResultBounds{Truncate: 3}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := statement{query: rawSQL(ctx, t, c.sql)}
			miss, missResult := runRecorded(ctx, t, st, r, canal, c.bounds)
			hit, hitResult := runRecorded(ctx, t, st, r, canal, c.bounds)
			if missResult.Hit || !hitResult.Hit {
				t.Fatalf("want a miss, then a hit: %+v, then %+v", missResult, hitResult)
			}
			if !bytes.Equal(hit.header, miss.header) {
				t.Errorf("the header:\n miss %s\n hit  %s", miss.header, hit.header)
			}
			if !bytes.Equal(hit.rows, miss.rows) {
				t.Errorf("the rows:\n miss %s\n hit  %s", miss.rows, hit.rows)
			}

			e, ok, err := qc.store.Lookup(ctx, querycache.Key(missResult.Key), time.Now())
			if err != nil || !ok {
				t.Fatalf("the entry: %v, %v", ok, err)
			}
			var lake recorded
			if err := query.CacheReadQuery(e.LakeKey()).Stream(ctx, canal, c.bounds, lake.handler()); err != nil {
				t.Fatalf("read the entry: %v", err)
			}
			if got := asMissed(t, lake.header, miss.header); !bytes.Equal(got, miss.header) {
				t.Errorf("canal-lake's own header, but for the known differences:\n miss %s\n lake %s", miss.header, got)
			}
		})
	}
}

// startCanal starts canal-lake, and canal-query writing to it, under the
// test's temp folder.
func startCanal(ctx context.Context, t *testing.T) *canalquery.Client {
	t.Helper()
	cfg := sidecar.Config{StderrWriter: logWriter{t}}
	lake, err := canallake.New(cfg, filepath.Join(t.TempDir(), "lake"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lake.Close(context.WithoutCancel(ctx)) })
	if err := lake.Start(ctx); err != nil {
		t.Fatal(err)
	}
	cfg.CanalLakeSocket = lake.SocketPath()
	cq := canalquery.New(cfg)
	if err := cq.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cq.Close(context.WithoutCancel(ctx)) })
	return cq.Client().ForTenant("acme")
}

// rawSQL is sql, as written, on an in-memory DuckDB.
func rawSQL(ctx context.Context, t *testing.T, sql string) query.DataSourceQuery {
	t.Helper()
	sources := datasource.Set{"w": {Name: "w", DBType: "duckdb", Connection: map[string]any{}, Identity: "parity"}}
	q, err := query.InDataSourceUngoverned(ctx, dataperm.Ungoverned(), sources, "w", sql)
	if err != nil {
		t.Fatal(err)
	}
	return q.DataSourceQuery
}

// recorded is a streamed result: its header, and its rows one per line, as
// canal-query wrote them.
type recorded struct {
	header []byte
	rows   []byte
}

func (rec *recorded) handler() canalquery.Handler {
	return canalquery.Handler{
		Header: func(h canalquery.Header) error {
			b, err := jsonkit.Marshal(h)
			rec.header = b
			return err
		},
		Row: func(row canalquery.Row) error {
			rec.rows = append(append(rec.rows, row...), '\n')
			return nil
		},
	}
}

// runRecorded runs st through the cache, as a query does.
func runRecorded(ctx context.Context, t *testing.T, st statement, r queryRun, canal *canalquery.Client, bounds query.ResultBounds) (recorded, CacheResult) {
	t.Helper()
	var rec recorded
	res, err := st.runCached(ctx, r, newTracker(sink{status: func(string) error { return nil }}), canal, bounds, rec.handler())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return rec, res
}

// asMissed is canal-lake's header with the differences anfra knows of, and
// restores from the stored header, undone:
//   - canal-lake names the columns hcol0, hcol1 and so on;
//   - its types are Arrow's labels (int32), not the warehouse's names (INTEGER);
//   - it labels a timestamp's zone Etc/UTC where the miss said UTC.
//
// A name that isn't hcolN is a difference not known, so the header is then
// left as canal-lake gave it.
func asMissed(t *testing.T, header, missed []byte) []byte {
	t.Helper()
	var h, m canalquery.Header
	if err := jsonkit.Unmarshal(header, &h); err != nil {
		t.Fatal(err)
	}
	if err := jsonkit.Unmarshal(missed, &m); err != nil {
		t.Fatal(err)
	}
	if len(h.Fields) != len(m.Fields) {
		return header
	}
	for i, f := range h.Fields {
		if f != fmt.Sprintf("hcol%d", i) {
			return header
		}
	}
	h.Fields, h.Types = m.Fields, m.Types
	b, err := jsonkit.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(strings.NewReplacer(`"timezone":"Etc/UTC"`, `"timezone":"UTC"`, "tz=Etc/UTC", "tz=UTC").Replace(string(b)))
}

// logWriter is the sidecars' output, in the test's log.
type logWriter struct{ t *testing.T }

func (w logWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}
