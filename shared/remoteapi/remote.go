package remoteapi

import (
	"crypto/sha1" //nolint:gosec // G505: git names blobs by SHA-1; BlobID computes such names, nothing secret
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Remote is where a repo publishes: an org on an anfra remote server, named by
// the org's address there. Not a git remote.
type Remote struct {
	Origin string // scheme and host: https://bi.example.com
	Org    string // the org's slug: acme
}

// org is an org's slug, as servers name orgs in their addresses.
var org = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// ParseRemote reads a Remote from the org's address,
// https://bi.example.com/o/acme: the address of the org's pages, which is what a
// person copies. It must be
// https, except on a loopback host, since a publish carries a credential.
func ParseRemote(raw string) (Remote, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return Remote{}, fmt.Errorf("%q is not a URL: %w", raw, err)
	}
	slug, ok := strings.CutPrefix(strings.TrimSuffix(u.Path, "/"), "/o/")
	switch {
	case u.Scheme != "https" && u.Scheme != "http", u.Host == "":
		return Remote{}, fmt.Errorf("%q is not an http(s) address: give the org's address, like https://bi.example.com/o/acme", raw)
	case u.User != nil, u.RawQuery != "", u.Fragment != "":
		return Remote{}, fmt.Errorf("%q has more than an address: give the org's address alone, like https://bi.example.com/o/acme", raw)
	case !ok || !org.MatchString(slug):
		return Remote{}, fmt.Errorf("%q is not an org's address: it ends in /o/<org>, like https://bi.example.com/o/acme", raw)
	case u.Scheme == "http" && !loopback(u.Hostname()):
		return Remote{}, fmt.Errorf("%q is not https: a publish carries your credential, so only a server on this machine may use http", raw)
	}
	return Remote{Origin: u.Scheme + "://" + u.Host, Org: slug}, nil
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// String is the org's address.
func (r Remote) String() string { return r.Origin + "/o/" + r.Org }

// Endpoint is where the anfra remote server serves op for the org.
func (r Remote) Endpoint(op string) string { return r.Origin + "/api/o/" + r.Org + "/" + op }

// BlobID is content's git blob id: SHA-1 over "blob <size>\0" and the content,
// as git names it with no filters applied. It is what a publish names a file's
// content by, and what the server answers for content it stores.
func BlobID(content []byte) string {
	h := sha1.New() //nolint:gosec // G401: git's object naming
	h.Write([]byte("blob " + strconv.Itoa(len(content)) + "\x00"))
	h.Write(content)
	return hex.EncodeToString(h.Sum(nil))
}
