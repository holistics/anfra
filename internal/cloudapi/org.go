package cloudapi

import (
	"errors"
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
		return "", errors.New("invalid server origin: provide an HTTPS origin")
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	if (u.Scheme != "https" && u.Scheme != "http") || host == "" || u.User != nil ||
		u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") {
		return "", errors.New("invalid server origin: provide only an HTTPS scheme and host")
	}
	if strings.ContainsAny(host, "%\\") {
		return "", errors.New("invalid server origin: scoped or escaped host")
	}
	if net.ParseIP(host) == nil && !hostnamePattern.MatchString(host) {
		return "", errors.New("invalid server origin: expected an ASCII hostname or literal IP")
	}
	if u.Scheme == "http" && host != "127.0.0.1" && host != "::1" {
		return "", errors.New("invalid server origin: HTTP requires literal 127.0.0.1 or [::1]")
	}
	port := u.Port()
	if strings.HasSuffix(u.Host, ":") {
		return "", errors.New("invalid server origin: empty port")
	}
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", errors.New("invalid server origin: port must be between 1 and 65535")
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
	if err != nil || u == nil || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") || u.RawPath != "" {
		return Org{}, errors.New("invalid org address: provide https://<server>/o/<org> without credentials, query or fragment")
	}
	slug, ok := strings.CutPrefix(strings.TrimSuffix(u.Path, "/"), "/o/")
	if !ok || !slugPattern.MatchString(slug) {
		return Org{}, errors.New("invalid org address: expected /o/<org> with a lowercase DNS-label org slug")
	}
	origin, err := ParseOrigin(u.Scheme + "://" + u.Host)
	if err != nil {
		return Org{}, err
	}
	return Org{Origin: origin, Slug: slug}, nil
}

// String is the org's canonical address.
func (o Org) String() string { return o.Origin + "/o/" + o.Slug }

// Endpoint is where the server serves op for this org.
func (o Org) Endpoint(op string) string { return o.Origin + "/api/o/" + o.Slug + "/" + op }
