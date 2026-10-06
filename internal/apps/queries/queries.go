// Package queries runs a Data App's queries and suggestions on anfra, and validates the AML.
package queries

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/holistics/anfra/internal/apps/anfra"
)

// Failure is a query anfra rejected or failed to run; it reaches the Data App as a QueryError.
type Failure struct{ Message string }

func (f *Failure) Error() string { return f.Message }

// Request is the SDK's BackendQueryRequest.
type Request struct {
	Dataset string          `json:"dataset"`
	AQL     string          `json:"aql"`
	Input   json.RawMessage `json:"input"`
	// Page and PageSize come together, and only for a query that declares a page size; 0 means unpaged.
	Page      int    `json:"page,omitempty"`
	PageSize  int    `json:"pageSize,omitempty"`
	Timezone  string `json:"timezone,omitempty"`
	BustCache bool   `json:"bustCache,omitempty"`
}

// Result is the SDK's BackendQueryResult.
type Result struct {
	Columns []json.RawMessage   `json:"columns"`
	Values  [][]json.RawMessage `json:"values"`
	Meta    Meta                `json:"meta"`
	Debug   Debug               `json:"debug"`
}

// Meta reports the page only for a paged query.
type Meta struct {
	Page     int `json:"page,omitempty"`
	PageSize int `json:"pageSize,omitempty"`
	NumRows  int `json:"numRows"`
}

type Debug struct {
	ExecutedAQL string `json:"executedAql"`
	ExecutedSQL string `json:"executedSql"`
	FromCache   bool   `json:"fromCache"`
	ExecutedAt  string `json:"executedAt"`
}

type queryData struct {
	SQL     string            `json:"sql"`
	AQL     string            `json:"aql"`
	Columns []json.RawMessage `json:"columns"`
	Result  *struct {
		Fields  []string            `json:"fields"`
		Records [][]json.RawMessage `json:"records"`
	} `json:"result"`
	// Present instead when the AQL didn't type-check.
	Diagnostics []struct {
		Message string `json:"message"`
		Line    int    `json:"line"`
		Column  int    `json:"column"`
	} `json:"diagnostics"`
}

func diagnosticsMessage(d queryData) string {
	var lines []string
	for _, diag := range d.Diagnostics {
		switch {
		case diag.Line > 0 && diag.Column > 0:
			lines = append(lines, fmt.Sprintf("line %d:%d: %s", diag.Line, diag.Column, diag.Message))
		case diag.Line > 0:
			lines = append(lines, fmt.Sprintf("line %d: %s", diag.Line, diag.Message))
		default:
			lines = append(lines, diag.Message)
		}
	}
	return strings.Join(lines, "\n")
}

// columnsFor orders anfra's column metadata the way the rows hold it. A column anfra couldn't
// describe (any query shape but an explore) is treated as adhoc, which keeps it out of
// cross-filtering.
func columnsFor(fields []string, described []json.RawMessage) []json.RawMessage {
	byName := map[string]json.RawMessage{}
	for _, c := range described {
		var named struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(c, &named) == nil {
			byName[named.Name] = c
		}
	}
	out := make([]json.RawMessage, 0, len(fields))
	for _, name := range fields {
		if c, ok := byName[name]; ok {
			out = append(out, c)
			continue
		}
		adhoc, _ := json.Marshal(map[string]any{
			"name": name, "fieldName": name, "label": name, "adhoc": true, "isMeasure": false,
		})
		out = append(out, adhoc)
	}
	return out
}

// Run runs one SDK query on anfra: the AQL, its Query Input and its Execution Options.
func Run(ctx context.Context, a *anfra.Anfra, req Request) (*Result, error) {
	args := map[string]any{
		"dataset": req.Dataset,
		"aql":     req.AQL,
	}
	// Unpaged unless the query declares a page size: anfra then returns every row.
	if req.PageSize > 0 {
		args["page"] = req.Page
		args["page-size"] = req.PageSize
	}
	if len(req.Input) > 0 && string(req.Input) != "null" {
		args["input"] = req.Input
	}
	if req.Timezone != "" {
		args["timezone"] = req.Timezone
	}

	status, raw, err := a.Call(ctx, "query", args)
	if err != nil {
		var callErr *anfra.CallError
		if errors.As(err, &callErr) {
			return nil, &Failure{Message: callErr.Message}
		}
		return nil, err
	}
	var data queryData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("anfra query answered %s: %w", raw, err)
	}
	if status == "invalid" || data.Result == nil {
		if msg := diagnosticsMessage(data); msg != "" {
			return nil, &Failure{Message: msg}
		}
		return nil, &Failure{Message: "anfra rejected the query."}
	}

	values := data.Result.Records
	if values == nil {
		values = [][]json.RawMessage{}
	}
	return &Result{
		Columns: columnsFor(data.Result.Fields, data.Columns),
		Values:  values,
		Meta:    Meta{Page: req.Page, PageSize: req.PageSize, NumRows: len(values)},
		// A local demo has one reader, who may see everything.
		Debug: Debug{ExecutedAQL: data.AQL, ExecutedSQL: data.SQL, ExecutedAt: time.Now().UTC().Format(time.RFC3339Nano)},
	}, nil
}

// SuggestionRequest is the SDK's FieldSuggestionsRequest.
type SuggestionRequest struct {
	Dataset string `json:"dataset"`
	Model   string `json:"model"`
	Field   string `json:"field"`
	Q       string `json:"q"`
}

// SuggestionLimit is how many values a filter control is offered at most.
const SuggestionLimit = 100

// Suggest offers a field-backed filter its field's values: one distinct-values explore through the
// same Query Input path as any query, sorted and capped. Text the reader typed narrows a text
// field, case-insensitively. The caller must have checked the field exists: it is spliced into AQL.
func Suggest(ctx context.Context, a *anfra.Anfra, req SuggestionRequest, fieldType string) ([]json.RawMessage, error) {
	field := req.Model + "." + req.Field
	filters := []any{}
	if q := strings.TrimSpace(req.Q); q != "" && fieldType == "text" {
		filters = append(filters, map[string]any{"field": field, "operator": "contains", "values": []string{q}})
	}
	input, _ := json.Marshal(map[string]any{
		"filters":    filters,
		"conditions": []any{},
		"sorts":      []any{map[string]string{"field": "value", "direction": "asc"}},
		"dateDrills": []any{},
	})
	result, err := Run(ctx, a, Request{
		Dataset:  req.Dataset,
		AQL:      fmt.Sprintf("explore { dimensions { value: %s } }", field),
		Input:    input,
		Page:     1,
		PageSize: SuggestionLimit,
	})
	if err != nil {
		return nil, err
	}
	values := []json.RawMessage{}
	for _, row := range result.Values {
		// Strings, numbers and booleans only: no nulls, nothing structured.
		if len(row) == 0 || len(row[0]) == 0 {
			continue
		}
		switch c := row[0][0]; {
		case c == '"', c == '-', c >= '0' && c <= '9', c == 't', c == 'f':
			values = append(values, row[0])
		}
	}
	return values, nil
}

// Problem is one AML problem, as the Shell's banner shows it.
type Problem struct {
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Message string `json:"message"`
}

// Validate returns the AML problems that break the Data Folder: files that don't compile, and
// error-severity findings. Warnings are left out of the banner.
func Validate(ctx context.Context, a *anfra.Anfra) ([]Problem, error) {
	_, raw, err := a.Call(ctx, "validate", nil)
	if err != nil {
		return nil, err
	}
	var data struct {
		CompileErrors []struct {
			FilePath string `json:"filePath"`
			Message  string `json:"message"`
			Row      int    `json:"row"`
			Col      int    `json:"col"`
		} `json:"compileErrors"`
		Reports []struct {
			FilePath string `json:"filePath"`
			Message  string `json:"message"`
			Severity string `json:"severity"`
		} `json:"reports"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	// anfra reports paths relative to the Data Folder, with a leading `/`.
	file := func(p string) string { return strings.TrimLeft(p, "/") }
	problems := []Problem{}
	for _, e := range data.CompileErrors {
		problems = append(problems, Problem{File: file(e.FilePath), Line: e.Row, Column: e.Col, Message: e.Message})
	}
	for _, r := range data.Reports {
		if r.Severity == "error" {
			problems = append(problems, Problem{File: file(r.FilePath), Message: r.Message})
		}
	}
	return problems, nil
}
