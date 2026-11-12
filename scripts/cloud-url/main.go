// cloud-url checks the cloud servers and support email a CLI build embeds.
//
// By default it reads them from the environment, ANFRA_CLOUD_URL and
// ANFRA_CLOUD_SUPPORT_EMAIL, and prints the servers in their canonical form
// (cloudapi.ParseCloudServers). With -manifest, it reads a release's from
// manifest.yml and prints both as NAME=value lines, for $GITHUB_ENV.
package main

import (
	"errors"
	"flag"
	"fmt"
	"net/mail"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/holistics/anfra/internal/cloudapi"
)

func main() {
	manifestPath := flag.String("manifest", "", "read the cloud servers and support email from this manifest.yml")
	flag.Parse()
	if *manifestPath != "" {
		servers, email, err := readManifest(*manifestPath)
		if err != nil {
			fail(*manifestPath, err)
		}
		fmt.Printf("ANFRA_CLOUD_URL=%s\nANFRA_CLOUD_SUPPORT_EMAIL=%s\n", servers, email)
		return
	}
	servers, err := checkServers(os.Getenv("ANFRA_CLOUD_URL"))
	if err != nil {
		fail("ANFRA_CLOUD_URL", err)
	}
	if err := checkSupportEmail(os.Getenv("ANFRA_CLOUD_SUPPORT_EMAIL")); err != nil {
		fail("ANFRA_CLOUD_SUPPORT_EMAIL", err)
	}
	fmt.Println(servers)
}

func fail(source string, err error) {
	fmt.Fprintln(os.Stderr, source+":", err)
	os.Exit(1)
}

var serverIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

var serverFields = []string{"id", "label", "url"}

// readManifest returns a manifest's cloud_servers, in ANFRA_CLOUD_URL's format,
// and its cloud_support_email.
func readManifest(path string) (servers, email string, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	var manifest struct {
		CloudServers      []map[string]string `yaml:"cloud_servers"`
		CloudSupportEmail string              `yaml:"cloud_support_email"`
	}
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		return "", "", err
	}
	entries := make([]string, 0, len(manifest.CloudServers))
	ids := map[string]bool{}
	for i, server := range manifest.CloudServers {
		where := fmt.Sprintf("cloud_servers entry %d", i+1)
		for field := range server {
			if !slices.Contains(serverFields, field) {
				return "", "", fmt.Errorf("%s has an unknown field, %q. A server has an id, a label, and a url if it's available", where, field)
			}
		}
		id, label := server["id"], server["label"]
		if !serverIDPattern.MatchString(id) {
			return "", "", fmt.Errorf("%s needs an id of lowercase letters, digits and hyphens, such as us", where)
		}
		if ids[id] {
			return "", "", fmt.Errorf("%s uses the id %q, which an earlier server already has. Give each server its own id", where, id)
		}
		ids[id] = true
		if strings.TrimSpace(label) == "" {
			return "", "", fmt.Errorf("%s (%s) needs a label, such as United States", where, id)
		}
		if strings.ContainsAny(label, ";=") {
			return "", "", fmt.Errorf("%s (%s) has a label with a \";\" or an \"=\", which separate servers in ANFRA_CLOUD_URL. Remove it from the label", where, id)
		}
		entry := label
		if server["url"] != "" {
			entry += "=" + server["url"]
		}
		entries = append(entries, entry)
	}
	if servers, err = checkServers(strings.Join(entries, ";")); err != nil {
		return "", "", fmt.Errorf("cloud_servers: %w", err)
	}
	if err := checkSupportEmail(manifest.CloudSupportEmail); err != nil {
		return "", "", fmt.Errorf("cloud_support_email: %w", err)
	}
	return servers, manifest.CloudSupportEmail, nil
}

// checkServers returns servers in their canonical form, if a build can embed them.
func checkServers(raw string) (string, error) {
	servers, err := cloudapi.ParseCloudServers(raw)
	if err != nil {
		return "", err
	}
	for _, server := range servers {
		// The build passes the value to the linker in single quotes, which can't hold one.
		if strings.Contains(server.Label, "'") {
			return "", fmt.Errorf("the label %q has a \"'\", which the build can't embed. Remove it from the label", server.Label)
		}
	}
	return cloudapi.FormatCloudServers(servers), nil
}

// checkSupportEmail refuses an email a build can't embed, or login can't put in a sentence.
func checkSupportEmail(email string) error {
	if email == "" {
		return nil
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || address.Name != "" {
		return fmt.Errorf("%q isn't an email address. Use a bare address, such as support@example.com", email)
	}
	if strings.Contains(email, "'") {
		return errors.New("the address has a \"'\", which the build can't embed")
	}
	return nil
}
