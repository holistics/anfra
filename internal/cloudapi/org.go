package cloudapi

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// Org is where a repo publishes: an org on a server, named by its address there,
// https://bi.example.com/o/acme.
type Org struct {
	Origin string // the server's scheme and host: https://bi.example.com
	Slug   string // the org's slug: acme
}

// slugPattern is an org's slug, as servers name orgs in their addresses.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ParseOrg reads an Org from the org's address,
// https://bi.example.com/o/acme: the address of the org's pages, which is what a
// person copies. It must be
// https, except on a loopback host, since a publish carries a credential.
func ParseOrg(raw string) (Org, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return Org{}, fmt.Errorf("%q is not a URL: %w", raw, err)
	}
	slug, ok := strings.CutPrefix(strings.TrimSuffix(u.Path, "/"), "/o/")
	switch {
	case u.Scheme != "https" && u.Scheme != "http", u.Host == "":
		return Org{}, fmt.Errorf("%q is not an http(s) address: give the org's address, like https://bi.example.com/o/acme", raw)
	case u.User != nil, u.RawQuery != "", u.Fragment != "":
		return Org{}, fmt.Errorf("%q has more than an address: give the org's address alone, like https://bi.example.com/o/acme", raw)
	case !ok || !slugPattern.MatchString(slug):
		return Org{}, fmt.Errorf("%q is not an org's address: it ends in /o/<org>, like https://bi.example.com/o/acme", raw)
	case u.Scheme == "http" && !loopback(u.Hostname()):
		return Org{}, fmt.Errorf("%q is not https: a publish carries your credential, so only a server on this machine may use http", raw)
	}
	return Org{Origin: u.Scheme + "://" + u.Host, Slug: slug}, nil
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// String is the org's address.
func (o Org) String() string { return o.Origin + "/o/" + o.Slug }

// Endpoint is where the server serves op for the org.
func (o Org) Endpoint(op string) string { return o.Origin + "/api/o/" + o.Slug + "/" + op }
