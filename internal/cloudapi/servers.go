package cloudapi

import (
	"errors"
	"fmt"
	"strings"
)

// CloudServer is one server bare `anfra auth login` offers. An empty Origin
// means the server isn't available yet, so login shows it but can't use it.
type CloudServer struct {
	Label  string
	Origin string
}

// ParseCloudServers reads the list of servers ANFRA_CLOUD_URL and the build's
// default hold. Entries are separated by ";", and each is one of:
//
//	United States=https://us.anfra.ai   a server with a label
//	Europe                              a server that isn't available yet
//	https://us.anfra.ai                 a server without a label
//
// A list may be empty, or have only servers that aren't available yet. Login
// then offers just organization URL entry.
func ParseCloudServers(raw string) ([]CloudServer, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') && raw[len(raw)-1] == raw[0] {
		// make and env files such as .env.local keep quotes as part of the value.
		return nil, errors.New("the value is in quotes, which are read as part of it. Remove the quotes, such as ANFRA_CLOUD_URL=United States=https://us.anfra.ai")
	}
	var servers []CloudServer
	labels, origins := map[string]bool{}, map[string]bool{}
	for entry := range strings.SplitSeq(raw, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		server, err := parseCloudServer(entry)
		if err != nil {
			return nil, fmt.Errorf("server %d: %w", len(servers)+1, err)
		}
		key := strings.ToLower(server.Label)
		if server.Label != "" && labels[key] {
			return nil, fmt.Errorf("server %d: the label %q is used twice. Give each server its own label", len(servers)+1, server.Label)
		}
		if server.Origin != "" && origins[server.Origin] {
			return nil, fmt.Errorf("server %d: %s is listed twice. List each server once", len(servers)+1, server.Origin)
		}
		if server.Label != "" {
			labels[key] = true
		}
		if server.Origin != "" {
			origins[server.Origin] = true
		}
		servers = append(servers, server)
	}
	return servers, nil
}

func parseCloudServer(entry string) (CloudServer, error) {
	// An entry whose "://" comes before any "=" is a URL alone, so an "=" in
	// its query stays part of the URL, which ParseOrigin then refuses.
	scheme, equals := strings.Index(entry, "://"), strings.Index(entry, "=")
	if scheme >= 0 && (equals < 0 || scheme < equals) {
		origin, err := ParseOrigin(entry)
		if err != nil {
			return CloudServer{}, err
		}
		return CloudServer{Origin: origin}, nil
	}
	label, address, hasAddress := strings.Cut(entry, "=")
	label = strings.TrimSpace(label)
	if hasAddress && label == "" {
		return CloudServer{}, errors.New("the label before \"=\" is empty. Write it as Label=https://server.example.com")
	}
	if hasAddress && strings.TrimSpace(address) == "" {
		return CloudServer{}, fmt.Errorf("%q has no address after \"=\". Add one, such as %s=https://server.example.com, or remove the \"=\" if the server isn't available yet", label, label)
	}
	if !hasAddress {
		return CloudServer{Label: label}, nil
	}
	origin, err := ParseOrigin(address)
	if err != nil {
		return CloudServer{}, err
	}
	return CloudServer{Label: label, Origin: origin}, nil
}

// FormatCloudServers writes servers in the format ParseCloudServers reads,
// with each address in its canonical form.
func FormatCloudServers(servers []CloudServer) string {
	entries := make([]string, len(servers))
	for i, server := range servers {
		switch {
		case server.Label == "":
			entries[i] = server.Origin
		case server.Origin == "":
			entries[i] = server.Label
		default:
			entries[i] = server.Label + "=" + server.Origin
		}
	}
	return strings.Join(entries, ";")
}
