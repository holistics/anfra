package query

import (
	"context"
	"encoding/csv"
	"encoding/json/jsontext"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/danielgtaylor/huma/v2"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/jsonkit"
)

// Export runs a query and answers its whole result as a file: a link to
// download it. See docs/designs/exports.md.
var Export = command.Define(command.Def[ExportInput, command.ExportLink]{
	Name:  "query.export",
	Short: "Export a query's whole result as a file: a link to download it",
	Long: "Export a query's whole result as a file: a link to download it.\n\n" +
		"The query runs to completion before the answer, so a query that fails fails here. " +
		"The CLI downloads the file, to stdout or --output; --link prints the answer instead.",
	// It changes something (a file, a link), but only adds it.
	NonDestructive: true,
	Timeout:        30 * time.Minute,
	Needs: func(in ExportInput) command.Sidecars {
		return command.Sidecars{Node: in.Lang != "sql", CanalQuery: true}
	},
	Check:  ExportInput.check,
	Errors: []apperr.AnyCode{query.QueryInvalid, errcode.QueryFailed, errcode.DataPermsUnenforceable, errcode.ExportsUnavailable},
	Run:    runExport,
})

// ExportInput is what query.export takes: a query and what shapes its run, as
// query takes them, but no page, since an export is the whole result; and the
// file's format.
type ExportInput struct {
	QueryInput
	Input         *anfranode.QueryTransforms `json:"input,omitempty" doc:"the Query Input: filters, conditions, sorts and date drills applied to the AQL before it compiles"`
	Timezone      string                     `json:"timezone,omitempty" doc:"the IANA time zone relative dates and date truncation use, such as Asia/Ho_Chi_Minh"`
	Format        string                     `json:"format,omitempty" enum:"csv" default:"csv" doc:"the file's format"`
	FormatOptions *ExportFormatOptions       `json:"format_options,omitempty" doc:"the format's options"`
	Filename      string                     `json:"filename,omitempty" doc:"the file's name, as a download saves it, with the format's extension added when it has none; when unset or blank, the dataset's or data source's name"`
}

func (ExportInput) TransformSchema(_ huma.Registry, s *huma.Schema) *huma.Schema {
	return command.ArgsSchema[ExportInput](s)
}

// ExportFormatOptions are a format's options. Each applies to the formats it names;
// given to another, it is refused (check).
type ExportFormatOptions struct {
	Header string `json:"header,omitempty" enum:"labels,names,none" doc:"csv: the header row: the columns' labels (the default; what readers see), their names (their keys in a query's answer, stable for programs), or none"`
	BOM    bool   `json:"bom,omitempty" doc:"csv: start the file with a UTF-8 byte-order mark, which Excel needs to read non-ASCII text; set it for a file a person will open"`
}

// check refuses what query refuses of the same query, a file name that cannot
// be one, and an option that does not belong to the format. CSV is the only
// format yet, so every option does.
func (in ExportInput) check() error {
	if err := in.runInput().check(); err != nil {
		return err
	}
	if name := strings.TrimSpace(in.Filename); name != "" {
		if msg := badFilename(name); msg != "" {
			return command.InvalidArg("filename", "invalid", msg)
		}
	}
	return nil
}

// filename is the file's name: the one asked for, its surrounding spaces
// dropped (check refused a bad one), or, when none is or it is blank, one made
// from what it exports; with the format's extension.
func (in ExportInput) filename(exported, ext string) string {
	name := strings.TrimSpace(in.Filename)
	if name == "" {
		name = safeFilename(exported)
	}
	if !strings.EqualFold(filepath.Ext(name), "."+ext) {
		name += "." + ext
	}
	return name
}

// runInput is the export's query as query takes it: no page.
func (in ExportInput) runInput() QueryRunInput {
	return QueryRunInput{QueryInput: in.QueryInput, Input: in.Input, Timezone: in.Timezone}
}

func (in ExportInput) options() ExportFormatOptions {
	if in.FormatOptions == nil {
		return ExportFormatOptions{}
	}
	return *in.FormatOptions
}

// rowsFunc runs the query, handing each row to each, and answers its fields.
type rowsFunc func(each func(canalquery.Row) error) ([]string, error)

// runExport decides what runs before any file exists, so a query that is
// refused, or does not compile, fails as it would in query; then writes the
// rows as they arrive.
func runExport(ctx context.Context, cc command.CommandContext, in ExportInput) (command.ExportLink, error) {
	var (
		name      string
		described []anfranode.ExploreColumn
		rows      rowsFunc
	)
	if in.Lang == "sql" {
		ds, err := in.dataSource(cc)
		if err != nil {
			return command.ExportLink{}, err
		}
		if err := command.RequireSidecars(cc, command.Sidecars{CanalQuery: true}); err != nil {
			return command.ExportLink{}, err
		}
		if err := command.RequireExports(cc); err != nil {
			return command.ExportLink{}, err
		}
		name = in.DataSource
		rows = func(each func(canalquery.Row) error) ([]string, error) {
			return query.ExecuteSQLEach(ctx, cc.Clients.CanalQuery, ds, in.Query, each)
		}
	} else {
		aql, limit, err := in.aql()
		if err != nil {
			return command.ExportLink{}, err
		}
		if err := command.RequireSidecars(cc, command.Sidecars{Node: true, CanalQuery: true}); err != nil {
			return command.ExportLink{}, err
		}
		if err := command.RequireExports(cc); err != nil {
			return command.ExportLink{}, err
		}
		compiled, err := compileAQL(ctx, cc, in.Dataset, aql, in.runInput().run())
		if err != nil {
			return command.ExportLink{}, err
		}
		name, described = in.Dataset, compiled.Columns
		rows = func(each func(canalquery.Row) error) ([]string, error) {
			return query.ExecuteEach(ctx, cc.Clients.CanalQuery, cc.Repo, compiled, limit, each)
		}
	}
	return writeCSV(ctx, cc.Exports, in.filename(name, "csv"), in.options(), described, rows)
}

// writeCSV writes the query's rows as a CSV file in store, and answers its link.
//
// The header row comes first, but the columns are known only when the last row
// has arrived (canal sends them at the end), so with a header the rows go to a
// spool file first, and follow the header into the file. Either way no row is
// held in memory.
func writeCSV(ctx context.Context, store command.ExportStore, filename string, opts ExportFormatOptions,
	described []anfranode.ExploreColumn, rows rowsFunc) (command.ExportLink, error) {
	header := opts.Header
	if header == "" {
		header = "labels"
	}
	contentType := "text/csv; charset=utf-8; header=present"
	if header == "none" {
		contentType = "text/csv; charset=utf-8; header=absent"
	}
	file, err := store.Create(ctx, filename, contentType)
	if err != nil {
		return command.ExportLink{}, fmt.Errorf("create the export's file: %w", err)
	}
	finished := false
	defer func() {
		if !finished {
			_ = file.Abort(context.WithoutCancel(ctx))
		}
	}()

	body := io.Writer(file)
	if header == "none" {
		if err := writeBOM(file, opts.BOM); err != nil {
			return command.ExportLink{}, err
		}
	} else {
		spool, err := os.CreateTemp("", "anfra-export-*.csv")
		if err != nil {
			return command.ExportLink{}, fmt.Errorf("create the export's spool file: %w", err)
		}
		defer spool.Close()
		// Unnamed as soon as it is open: the open file stays usable, and the
		// system frees it when it is closed, even by a crash, so nothing is
		// left to clean up. Where an open file cannot be removed (Windows), it
		// is removed when the export returns.
		if os.Remove(spool.Name()) != nil {
			defer os.Remove(spool.Name())
		}
		body = spool
	}

	w := csv.NewWriter(body)
	count := 0
	fields, err := rows(func(row canalquery.Row) error {
		cells, err := csvCells(row)
		if err != nil {
			return err
		}
		count++
		return w.Write(cells)
	})
	if err != nil {
		return command.ExportLink{}, failed(err)
	}
	if w.Flush(); w.Error() != nil {
		return command.ExportLink{}, fmt.Errorf("write the export's rows: %w", w.Error())
	}

	if spool, ok := body.(*os.File); ok {
		if err := writeBOM(file, opts.BOM); err != nil {
			return command.ExportLink{}, err
		}
		hw := csv.NewWriter(file)
		_ = hw.Write(headerRow(fields, described, header)) // its error is Error's, after Flush
		if hw.Flush(); hw.Error() != nil {
			return command.ExportLink{}, fmt.Errorf("write the export's header: %w", hw.Error())
		}
		if _, err := spool.Seek(0, io.SeekStart); err != nil {
			return command.ExportLink{}, fmt.Errorf("read the export's spool file: %w", err)
		}
		if _, err := io.Copy(file, spool); err != nil {
			return command.ExportLink{}, fmt.Errorf("copy the export's rows: %w", err)
		}
	}

	url, expiresAt, err := file.Done(ctx)
	finished = true
	if err != nil {
		return command.ExportLink{}, fmt.Errorf("finish the export's file: %w", err)
	}
	return command.ExportLink{URL: url, Filename: filename, Format: "csv", RowCount: count, ExpiresAt: expiresAt}, nil
}

func writeBOM(w io.Writer, bom bool) error {
	if !bom {
		return nil
	}
	if _, err := io.WriteString(w, "\xEF\xBB\xBF"); err != nil {
		return fmt.Errorf("write the export's byte-order mark: %w", err)
	}
	return nil
}

// csvCells is a row's cells as CSV writes them: a string as itself, null as an
// empty cell, a number or a boolean as written, with every digit, and an object
// or a list as its JSON.
func csvCells(row canalquery.Row) ([]string, error) {
	var values []jsontext.Value
	if err := jsonkit.Unmarshal(row, &values); err != nil {
		return nil, fmt.Errorf("read a row: %w", err)
	}
	cells := make([]string, len(values))
	for i, v := range values {
		switch v.Kind() {
		case 'n':
		case '"':
			if err := jsonkit.Unmarshal(v, &cells[i]); err != nil {
				return nil, fmt.Errorf("read a cell: %w", err)
			}
		default:
			cells[i] = string(v)
		}
	}
	return cells, nil
}

// headerRow is the header for the result's fields: each column's label, or its
// name.
func headerRow(fields []string, described []anfranode.ExploreColumn, header string) []string {
	cols := describeFields(fields, described)
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.Name
		if header == "labels" && c.Label != "" {
			out[i] = c.Label
		}
	}
	return out
}

// badFilename says why name cannot be a file's name, or "" when it can. The
// name is never a path on disk: the store keeps it beside the link, and a
// download's header carries it encoded, so it need only be one name, not a
// path, and no longer than a file system allows. What a system forbids in a
// name (a colon on Windows, say) is the browser's to replace where the file
// lands. Any script is welcome.
func badFilename(name string) string {
	switch {
	case strings.Trim(name, ". ") == "":
		return "a file name needs more than dots"
	case strings.ContainsAny(name, `/\`):
		return "a file name is one name, not a path: it cannot hold / or \\"
	case strings.ContainsFunc(name, unicode.IsControl):
		return "a file name cannot hold control characters"
	case len(name) > 255:
		return "a file name is at most 255 bytes"
	}
	return ""
}

// safeFilename makes a file name of what is exported: what badFilename refuses
// replaced, dots and spaces trimmed from its ends, and "export" when nothing
// is left.
func safeFilename(name string) string {
	safe := strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return '_'
		}
		return r
	}, name)
	safe = strings.Trim(safe, ". ")
	if strings.Trim(safe, "_") == "" || badFilename(safe) != "" {
		return "export"
	}
	return safe
}
