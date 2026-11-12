package cliui

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/holistics/anfra/internal/cloudapi"
)

func TestPickOrgKeepsAuthenticationAfterOrganizations(t *testing.T) {
	orgs := []OrgChoice{
		{Name: "Zulu", URL: "https://bi.example.com/o/zulu"},
		{Name: "Tango", URL: "https://bi.example.com/o/tango"},
		{Name: "Switch to another organization"},
		{Name: "Authenticate with another organization"},
	}
	for _, tc := range []struct {
		name  string
		keys  string
		index int
	}{
		{name: "first organization", keys: "\r", index: 1},
		{name: "second organization", keys: "\x1b[B\r", index: 0},
		{name: "switch before authentication", keys: "\x1b[B\x1b[B\r", index: 2},
		{name: "authentication last", keys: "\x1b[B\x1b[B\x1b[B\r", index: 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			index, address, err := PickOrg(ctx, strings.NewReader(tc.keys), io.Discard, "Choose where to publish", orgs, nil, false)
			if err != nil {
				t.Fatal(err)
			}
			if index != tc.index || address != "" {
				t.Fatalf("selection = (%d, %q), want (%d, empty address)", index, address, tc.index)
			}
		})
	}
}

func TestPickOrgSkipsUpcomingServers(t *testing.T) {
	servers := []cloudapi.CloudServer{
		{Label: "Europe"},
		{Label: "United States", Origin: "https://us.example.com"},
		{Label: "Asia Pacific"},
		{Label: "Middle East", Origin: "https://me.example.com"},
	}
	for _, tc := range []struct {
		name, keys, address string
	}{
		{name: "starts on the first available", keys: "\r", address: "https://us.example.com"},
		{name: "down past an upcoming", keys: "\x1b[B\r", address: "https://me.example.com"},
		{name: "up past an upcoming, and stays put with none above", keys: "\x1b[B\x1b[A\x1b[A\r", address: "https://us.example.com"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			index, address, err := PickOrg(ctx, strings.NewReader(tc.keys), io.Discard, "Authenticate with an organization", nil, &CloudChoices{Servers: servers}, true)
			if err != nil {
				t.Fatal(err)
			}
			if index != -1 || address != tc.address {
				t.Fatalf("selection = (%d, %q), want (-1, %q)", index, address, tc.address)
			}
		})
	}
}
