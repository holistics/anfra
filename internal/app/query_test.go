package app

import (
	"context"
	"strings"
	"testing"

	"github.com/holistics/anfra/internal/repo"
)

func TestQueryRunReadsExecutionOptions(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		page     int
		pageSize int
		timezone string
		wantErr  string
	}{
		{name: "nothing set", args: map[string]any{}},
		{name: "CLI strings", args: map[string]any{"page": "2", "page-size": "50", "timezone": " Asia/Tokyo "}, page: 2, pageSize: 50, timezone: "Asia/Tokyo"},
		{name: "/call numbers", args: map[string]any{"page": float64(3), "page-size": float64(10)}, page: 3, pageSize: 10},
		{name: "page size alone", args: map[string]any{"page-size": "25"}, page: 1, pageSize: 25},
		{name: "page without size", args: map[string]any{"page": "2"}, wantErr: "--page needs --page-size"},
		{name: "fractional page", args: map[string]any{"page": 1.5, "page-size": float64(10)}, wantErr: "invalid --page"},
		{name: "zero page size", args: map[string]any{"page-size": "0"}, wantErr: "invalid --page-size"},
		{name: "bad input", args: map[string]any{"input": "[]"}, wantErr: "invalid input"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, err := queryRun(tt.args)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			gotPage, gotSize := 0, 0
			if run.Pagination != nil {
				gotPage, gotSize = run.Pagination.Page, run.Pagination.PageSize
			}
			if gotPage != tt.page || gotSize != tt.pageSize || run.Timezone != tt.timezone {
				t.Errorf("got page %d size %d tz %q, want %d %d %q", gotPage, gotSize, run.Timezone, tt.page, tt.pageSize, tt.timezone)
			}
		})
	}
}

// Paging replaces the old `limit:` directive, so a query can't use both.
func TestQueryRejectsPagingWithLimitDirective(t *testing.T) {
	var cmd Command
	for _, c := range Commands {
		if c.Name == "query" {
			cmd = c
		}
	}
	_, err := cmd.Run(context.Background(), Clients{}, repo.Repo{}, map[string]any{
		"dataset":   "ecommerce",
		"aql":       "explore { dimensions { n: products.name } } limit: 5",
		"page-size": "10",
	})
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("err = %v, want a paging/limit conflict", err)
	}
}
