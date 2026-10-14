package cliui

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"
)

func TestPickOrgKeepsAuthenticationAfterOrganizations(t *testing.T) {
	orgs := []OrgChoice{
		{Name: "Zulu", URL: "https://bi.example.com/o/zulu"},
		{Name: "Tango", URL: "https://bi.example.com/o/tango"},
		{Name: "Authenticate with another organization"},
	}
	for _, tc := range []struct {
		name  string
		keys  string
		index int
	}{
		{name: "first organization", keys: "\r", index: 1},
		{name: "second organization", keys: "\x1b[B\r", index: 0},
		{name: "authentication last", keys: "\x1b[B\x1b[B\r", index: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			index, address, err := PickOrg(ctx, strings.NewReader(tc.keys), io.Discard, "Choose where to publish", orgs, "", false)
			if err != nil {
				t.Fatal(err)
			}
			if index != tc.index || address != "" {
				t.Fatalf("selection = (%d, %q), want (%d, empty address)", index, address, tc.index)
			}
		})
	}
}
