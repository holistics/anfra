package telemetry_test

import (
	"testing"

	"github.com/holistics/anfra/shared/telemetry"
)

// Each warehouse's literals are masked as it quotes them, and its
// identifiers, quoted or not, are kept.
func TestMaskSQLMasksEachWarehousesLiterals(t *testing.T) {
	for _, tc := range []struct {
		system, sql, want string
	}{
		{"postgresql", `SELECT "email" FROM t1 WHERE name = 'O''Brien' AND age > 30`, `SELECT "email" FROM t1 WHERE name = ? AND age > ?`},
		{"postgresql", `SELECT 'C:\' AS p, 'x' AS q`, `SELECT ? AS p, ? AS q`},
		{"postgresql", `SELECT E'it\'s' AS a`, `SELECT E? AS a`},
		{"postgresql", `SELECT $$a 'secret'$$, $tag$b$tag$, $1`, `SELECT ?, ?, $1`},
		{"postgresql", "SELECT 1.5e-3, .5, 0x1F -- a 'comment'\nFROM t", "SELECT ?, ?, ?  \nFROM t"},
		{"postgresql", `SELECT /* 'x' */ col_2 FROM s.t`, `SELECT   col_2 FROM s.t`},
		{"mysql", `SELECT * FROM t WHERE a = 'it\'s' AND b = "two" # tail`, `SELECT * FROM t WHERE a = ? AND b = ?  `},
		{"bigquery", "SELECT `p.d.t`.x FROM `p.d.t` WHERE s = '''multi\nline''' AND r = r'\\d' AND n = 42", "SELECT `p.d.t`.x FROM `p.d.t` WHERE s = ? AND r = r? AND n = ?"},
		{"bigquery", `SELECT "a string"`, `SELECT ?`},
		{"snowflake", `SELECT 'a\'b', "Col" FROM "DB"."T"`, `SELECT ?, "Col" FROM "DB"."T"`},
		{"sqlserver", `SELECT [order id] FROM [dbo].[t] WHERE x = N'é'`, `SELECT [order id] FROM [dbo].[t] WHERE x = N?`},
		{"duckdb", `SELECT 'unclosed`, `SELECT ?`},
		{"somewhere", `SELECT 'a\'b' , 'c'`, `SELECT ? , ?`},
	} {
		if got := telemetry.MaskSQL(tc.sql, tc.system); got != tc.want {
			t.Errorf("%s: MaskSQL(%q)\n got %q\nwant %q", tc.system, tc.sql, got, tc.want)
		}
	}
}

// An error's quoted strings and numbers are masked, its words kept.
func TestMaskErrorMasksQuotedStringsAndNumbers(t *testing.T) {
	for _, tc := range []struct{ msg, want string }{
		{`invalid date '2024-13-01'`, `invalid date '?'`},
		{`column "email" does not exist at line 3.`, `column "?" does not exist at line ?.`},
		{`can't parse value 12.5 for col_2`, `can't parse value ? for col_2`},
		{`users' table has “Bob”`, `users' table has “?”`},
		{`unclosed 'secret`, `unclosed '?`},
		{`Table proj.ds.t was not found`, `Table proj.ds.t was not found`},
	} {
		if got := telemetry.MaskError(tc.msg); got != tc.want {
			t.Errorf("MaskError(%q)\n got %q\nwant %q", tc.msg, got, tc.want)
		}
	}
}
