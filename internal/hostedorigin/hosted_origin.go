// Package hostedorigin holds the hosted server origin selected when the CLI is built.
package hostedorigin

import (
	"errors"
	"fmt"
	"strings"

	"github.com/holistics/anfra/internal/cloudapi"
)

// embeddedOrigin is set with -X by the build; runtime environment variables never select it.
var embeddedOrigin string

// HostedOrigin returns the canonical build-time hosted origin, or asks for an explicit org URL.
func HostedOrigin() (string, error) {
	if embeddedOrigin == "" {
		return "", errors.New("Provide an org URL.")
	}
	return ParseHostedOrigin(embeddedOrigin)
}

// ParseHostedOrigin validates build configuration: hosted origins require HTTPS, even on loopback.
func ParseHostedOrigin(raw string) (string, error) {
	origin, err := cloudapi.ParseOrigin(raw)
	if err != nil {
		return "", fmt.Errorf("ANFRA_HOSTED_ORIGIN: %w", err)
	}
	if !strings.HasPrefix(origin, "https://") {
		return "", errors.New("ANFRA_HOSTED_ORIGIN: require an HTTPS origin")
	}
	return origin, nil
}
