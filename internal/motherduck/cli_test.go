package motherduck

import (
	"context"
	"encoding/base64"
	"errors"
	"regexp"
	"strings"
	"testing"
)

// fakeCLI answers each call with out and err, recording the arguments.
type fakeCLI struct {
	calls [][]string
	out   string
	err   error
}

func (f *fakeCLI) cli() CLI {
	return CLI{run: func(_ context.Context, args ...string) ([]byte, error) {
		f.calls = append(f.calls, args)
		return []byte(f.out), f.err
	}}
}

// destructive is SQL that would change or remove what an account has.
var destructive = regexp.MustCompile(`(?i)\b(replace|drop|alter|delete|update|insert|truncate|detach|copy)\b`)

// Every statement anfra sends to MotherDuck only reads, or creates a database
// under a new name from a file attached read-only.
func TestStatementsNeverChangeWhatIsThere(t *testing.T) {
	f := &fakeCLI{out: `[{"n":"0"}]`}
	c := f.cli()
	if _, err := c.DatabaseExists(context.Background(), "northwind"); err != nil {
		t.Fatal(err)
	}
	f.out = ""
	if err := c.CreateDatabaseFrom(context.Background(), "northwind", "/data/it's here.duckdb"); err != nil {
		t.Fatal(err)
	}
	for _, call := range f.calls {
		sql := call[len(call)-1]
		if destructive.MatchString(sql) {
			t.Errorf("a statement that may change the account: %q", sql)
		}
	}
	upload := f.calls[1][len(f.calls[1])-1]
	if !regexp.MustCompile(`^ATTACH '/data/it''s here\.duckdb' AS anfra_upload_[0-9a-f]{16} \(READ_ONLY\); CREATE DATABASE northwind FROM anfra_upload_[0-9a-f]{16}$`).MatchString(upload) {
		t.Errorf("upload: %q", upload)
	}
	if !strings.Contains(strings.Join(f.calls[1], " "), "--timeout 86400") {
		t.Errorf("an upload limited to the CLI's default 120 seconds: %q", f.calls[1])
	}
}

// A name or path that could carry SQL is refused before anything is sent.
func TestRefusesNamesAndPathsItDoesNotMake(t *testing.T) {
	f := &fakeCLI{}
	c := f.cli()
	for _, name := range []string{"North", "a b", "x; DROP DATABASE y", "", "1abc", strings.Repeat("a", 64)} {
		if err := c.CreateDatabaseFrom(context.Background(), name, "/f.duckdb"); err == nil {
			t.Errorf("name %q taken", name)
		}
		if _, err := c.DatabaseExists(context.Background(), name); err == nil {
			t.Errorf("name %q looked up", name)
		}
	}
	for _, path := range []string{"relative.duckdb", "/a\nb.duckdb", "/a\x00b"} {
		if err := c.CreateDatabaseFrom(context.Background(), "ok", path); err == nil {
			t.Errorf("path %q taken", path)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("sent %q", f.calls)
	}
}

func TestDatabaseName(t *testing.T) {
	for in, want := range map[string]string{
		"northwind":             "northwind",
		"Northwind-2024 (copy)": "northwind_2024__copy",
		"2024":                  "db_2024",
		"--":                    "db",
		strings.Repeat("x", 80): strings.Repeat("x", 56),
	} {
		got := DatabaseName(in)
		if got != want {
			t.Errorf("DatabaseName(%q) = %q, want %q", in, got, want)
		}
		if !validName(got) || !validName(got+"_99") {
			t.Errorf("DatabaseName(%q) = %q, which with a suffix is not a valid name", in, got)
		}
	}
}

// The token is read from the pragma's one line; when it is not there, the error
// does not carry what the CLI said, which may hold it.
func TestSessionToken(t *testing.T) {
	f := &fakeCLI{out: "export motherduck_token=eyJ.a.b\nSuccess\n-------\n"}
	if got, err := f.cli().SessionToken(context.Background()); err != nil || got != "eyJ.a.b" {
		t.Errorf("token %q, %v", got, err)
	}
	f = &fakeCLI{out: "motherduck_token=eyJ.secret.part", err: errors.New("exit status 1: eyJ.secret.part")}
	if _, err := f.cli().SessionToken(context.Background()); err == nil || strings.Contains(err.Error(), "secret") {
		t.Errorf("err %v", err)
	}
}

func TestStatus(t *testing.T) {
	f := &fakeCLI{out: `{"authenticated":false,"token_source":"","user":"","organization_name":"","duckling_size":""}`}
	if s, err := f.cli().Status(context.Background()); err != nil || s.Authenticated {
		t.Errorf("signed out: %+v, %v", s, err)
	}
	f.out = `{"authenticated":true,"user":"ada","organization_name":"acme"}`
	if s, err := f.cli().Status(context.Background()); err != nil || s != (Status{Authenticated: true, User: "ada", Organization: "acme"}) {
		t.Errorf("signed in: %+v, %v", s, err)
	}
}

// A refused statement is a QueryError with DuckDB's message; a held file names
// its holder.
func TestQueryErrors(t *testing.T) {
	f := &fakeCLI{
		out: `{"success":false,"error":"IO Error: Could not set lock on file \"/d/n.duckdb\": Conflicting lock is held in /home/a/.anfra/sidecars/canal-query-1a2b (PID 4242) by user a. See also https://duckdb.org/docs/connect/concurrency"}`,
		err: errors.New("exit status 1"),
	}
	err := f.cli().CreateDatabaseFrom(context.Background(), "n", "/d/n.duckdb")
	holder, pid, locked := LockedBy(err)
	if !locked || holder != "/home/a/.anfra/sidecars/canal-query-1a2b" || pid != 4242 {
		t.Errorf("LockedBy(%v) = %q, %d, %v", err, holder, pid, locked)
	}
	f.out = `{"success":false,"error":"Binder Error: database with name \"n\" already exists"}`
	err = f.cli().CreateDatabaseFrom(context.Background(), "n", "/d/n.duckdb")
	var q *QueryError
	if !errors.As(err, &q) || !strings.Contains(q.Message, "already exists") {
		t.Errorf("err %v", err)
	}
	if _, _, locked := LockedBy(err); locked {
		t.Error("taken for a lock")
	}
}

// Rows are the last statement's: one JSON line per statement that answers some.
func TestQueryAnswersTheLastStatementsRows(t *testing.T) {
	f := &fakeCLI{out: "[{\"x\":1}]\n[{\"n\":\"3\"}]\n"}
	rows, err := f.cli().query(context.Background(), "SELECT 1 AS x; SELECT 3 AS n")
	if err != nil || len(rows) != 1 || rows[0]["n"] != "3" {
		t.Errorf("rows %v, %v", rows, err)
	}
}

// A pasted token is read-only when it says it is a read-scaling one; anything
// else, or no MotherDuck token at all, is not.
func TestReadOnly(t *testing.T) {
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	tok := func(claims string) string { return enc(`{"alg":"HS256"}`) + "." + enc(claims) + "." + enc("sig") }
	for token, want := range map[string]bool{
		tok(`{"tokenType":"read_scaling","readOnly":true}`):  true,
		tok(`{"tokenType":"read_write","readOnly":false}`):   false,
		tok(`{"tokenType":"read_scaling","readOnly":false}`): false,
	} {
		if got, err := ReadOnly(token); err != nil || got != want {
			t.Errorf("ReadOnly(%s) = %v, %v", token, got, err)
		}
	}
	for _, token := range []string{"", "hunter2", "a.b.c", tok(`{"readOnly":true}`)} {
		if _, err := ReadOnly(token); err == nil {
			t.Errorf("%q taken for a MotherDuck token", token)
		}
	}
}
