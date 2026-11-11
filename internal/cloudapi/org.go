package cloudapi

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Org is one org on its canonical server origin, never a credential.
type Org struct {
	Origin string
	Slug   string
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
var hostnamePattern = regexp.MustCompile(`^[a-z0-9.-]+$`)

// ParseOrigin canonicalizes a server origin; HTTP is allowed only on literal loopback.
func ParseOrigin(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return "", errors.New("the address isn't a valid URL. Use the server's address, such as https://bi.example.com")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if (u.Scheme != "https" && u.Scheme != "http") || host == "" {
		return "", errors.New("the address must start with https:// and the server's host name, such as https://bi.example.com")
	}
	if u.User != nil || u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") {
		// Not echoed: it may hold credentials.
		return "", errors.New("the server's address can't have credentials, a path, a query, or a fragment. Use just its https:// address, such as https://bi.example.com")
	}
	if strings.ContainsAny(host, "%\\") {
		return "", errors.New(`the host name can't have a "%" or a "\"`)
	}
	if net.ParseIP(host) == nil && !hostnamePattern.MatchString(host) {
		return "", errors.New("the host name must be an IP address, or have only ASCII letters, digits, dots, and hyphens")
	}
	if u.Scheme == "http" && host != "127.0.0.1" && host != "::1" {
		return "", errors.New("the address must use https://, unless it's http://127.0.0.1 or http://[::1] for a server on this machine")
	}
	port := u.Port()
	if strings.HasSuffix(u.Host, ":") {
		return "", errors.New(`the port is empty. Remove the ":", or add the port`)
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", errors.New("the port must be a number from 1 to 65535")
		}
		port = strconv.Itoa(n)
	}
	if (u.Scheme == "https" && port == "443") || (u.Scheme == "http" && port == "80") {
		port = ""
	}
	if port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return u.Scheme + "://" + host, nil
}

// ParseOrg reads the copied org address, rejecting encoded paths and unsafe origins.
func ParseOrg(raw string) (Org, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		// Not echoed: it may hold credentials.
		return Org{}, errors.New("the address isn't a valid URL. Use the org's address, such as https://bi.example.com/o/acme")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") || u.RawPath != "" {
		// Not echoed: it may hold credentials.
		return Org{}, errors.New("an org address can't have credentials, a query, or a fragment. Use just the org's address, such as https://bi.example.com/o/acme")
	}
	slug, ok := strings.CutPrefix(strings.TrimSuffix(u.Path, "/"), "/o/")
	// An org's name is lowercase, so /o/Acme is acme, as the host's case is
	// ignored too.
	slug = strings.ToLower(slug)
	if !ok || !slugPattern.MatchString(slug) {
		return Org{}, fmt.Errorf("%q isn't an org address. Use the org's address, such as https://bi.example.com/o/acme", raw)
	}
	origin, err := ParseOrigin(u.Scheme + "://" + u.Host)
	if err != nil {
		return Org{}, err
	}
	return Org{Origin: origin, Slug: slug}, nil
}

// ParseAddress reads an address a person gave: an org's, any page under it, or
// only its server's. It returns the server's origin, and the org's slug when
// the address names one, or "". What follows the org in the address, its path,
// query and fragment, is dropped unread: a page's address copied from a
// browser works. Addresses anfra keeps, or a server returns, are parsed with
// ParseOrg, which allows only the org's own.
//
// An org named by its host, such as https://acme.anfra.ai, would need asking
// its server: here it reads as a server's address.
func ParseAddress(raw string) (origin, slug string, err error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		// Not echoed: it may hold credentials.
		return "", "", errors.New("the address isn't a valid URL. Use the org's address, such as https://bi.example.com/o/acme")
	}
	if u.User != nil {
		// Not echoed: it holds a credential.
		return "", "", errors.New("an org address can't have credentials. Use just the org's address, such as https://bi.example.com/o/acme")
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", errors.New("the address must start with https:// and the server's host name, such as https://bi.example.com/o/acme")
	}
	if origin, err = ParseOrigin(u.Scheme + "://" + u.Host); err != nil {
		return "", "", err
	}
	segments := strings.Split(strings.TrimPrefix(u.EscapedPath(), "/"), "/")
	if len(segments) < 2 || segments[0] != "o" {
		return origin, "", nil
	}
	// An org's name is lowercase, so /o/Acme is acme, as the host's case is
	// ignored too. An escaped name, such as %61cme, isn't one.
	slug = strings.ToLower(segments[1])
	if !slugPattern.MatchString(slug) {
		return "", "", fmt.Errorf("the address doesn't name an org: an org's name, after /o/, has only letters, digits, and hyphens. Use the org's address, such as %s/o/acme", origin)
	}
	return origin, slug, nil
}

// ParseOrgAddress reads an org's address a person gave, or any page's under
// it (ParseAddress).
func ParseOrgAddress(raw string) (Org, error) {
	origin, slug, err := ParseAddress(raw)
	if err != nil {
		return Org{}, err
	}
	if slug == "" {
		return Org{}, fmt.Errorf("%s doesn't name an org. Use the org's address, such as %s/o/acme: open the org in a browser, and copy the address of any of its pages", origin, origin)
	}
	return Org{Origin: origin, Slug: slug}, nil
}

// String is the org's canonical address.
func (o Org) String() string { return o.Origin + "/o/" + o.Slug }

// Endpoint is where the server serves op for this org.
func (o Org) Endpoint(op string) string { return o.Origin + "/api/o/" + o.Slug + "/" + op }
