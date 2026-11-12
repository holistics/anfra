package cloudapi

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseCloudServers(t *testing.T) {
	for _, tc := range []struct {
		name, raw string
		want      []CloudServer
		wantError string
	}{
		{name: "empty"},
		{name: "a URL alone", raw: "HTTPS://Cloud.Example.COM:443/", want: []CloudServer{{Origin: "https://cloud.example.com"}}},
		{
			name: "labels and upcoming", raw: " United States = https://us.example.com ; Europe ;https://other.example.com;",
			want: []CloudServer{{Label: "United States", Origin: "https://us.example.com"}, {Label: "Europe"}, {Origin: "https://other.example.com"}},
		},
		{name: "only upcoming", raw: "Europe;Asia Pacific", want: []CloudServer{{Label: "Europe"}, {Label: "Asia Pacific"}}},
		{name: "invalid address", raw: "United States=https://us.example.com/path", wantError: "server 1: the server's address can't have"},
		{name: "an = in a URL alone's query", raw: "https://us.example.com/?next=http://127.0.0.1:1", wantError: "server 1: the server's address can't have"},
		{name: "in quotes", raw: `"United States=https://us.example.com"`, wantError: "the value is in quotes"},
		{name: "empty label", raw: "=https://us.example.com", wantError: "label before"},
		{name: "repeated label", raw: "Europe=https://eu.example.com;europe", wantError: `"europe" is used twice`},
		{name: "repeated server", raw: "US=https://us.example.com;Also US=https://US.example.com", wantError: "https://us.example.com is listed twice"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			servers, err := ParseCloudServers(tc.raw)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("error = %v, want %q", err, tc.wantError)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(servers, tc.want) {
				t.Fatalf("servers = %#v, %v; want %#v", servers, err, tc.want)
			}
		})
	}
}

func TestFormatCloudServersReadsBack(t *testing.T) {
	raw := "United States=https://us.example.com;Europe;https://other.example.com"
	servers, err := ParseCloudServers(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := FormatCloudServers(servers); got != raw {
		t.Fatalf("formatted = %q, want %q", got, raw)
	}
}
