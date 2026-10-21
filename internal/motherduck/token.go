package motherduck

import (
	"encoding/base64"
	"errors"
	"strings"

	"github.com/holistics/anfra/shared/jsonkit"
)

// ReadOnly reports whether token, a MotherDuck token someone pasted, only
// reads, by what it says of itself: its type, in its claims. err is a token that
// is not MotherDuck's. The claims are not verified: MotherDuck enforces what a
// token may do; this only keeps one that says it writes from an org.
func ReadOnly(token string) (bool, error) {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return false, errors.New("not a MotherDuck token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(parts[1], "="))
	if err != nil {
		return false, errors.New("not a MotherDuck token")
	}
	var claims struct {
		TokenType string `json:"tokenType"`
		ReadOnly  bool   `json:"readOnly"`
	}
	if err := jsonkit.Unmarshal(payload, &claims); err != nil || claims.TokenType == "" {
		return false, errors.New("not a MotherDuck token")
	}
	return claims.TokenType == "read_scaling" && claims.ReadOnly, nil
}
