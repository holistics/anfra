package requestid_test

import (
	"context"
	"regexp"
	"testing"

	"github.com/holistics/anfra/shared/requestid"
)

func TestNewHasTheDocumentedShape(t *testing.T) {
	shape := regexp.MustCompile(`^req_[a-z2-7]{26}$`) // 128 bits: 26 base32 characters
	seen := map[string]bool{}
	for range 100 {
		id := requestid.New()
		if !shape.MatchString(id) {
			t.Fatalf("%q does not look like req_ plus 128 bits of base32", id)
		}
		if seen[id] {
			t.Fatalf("%q minted twice", id)
		}
		seen[id] = true
	}
}

func TestContextRoundTrip(t *testing.T) {
	if got := requestid.From(context.Background()); got != "" {
		t.Errorf("an empty context carries %q", got)
	}
	ctx := requestid.With(context.Background(), "req_x")
	if got := requestid.From(ctx); got != "req_x" {
		t.Errorf("From = %q, want req_x", got)
	}
}
