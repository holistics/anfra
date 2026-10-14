package apikit_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/holistics/anfra/shared/apikit"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/httpkit"
)

type headerDetails struct {
	Reason string `json:"reason"`
}

var headerDenied = apperr.DefinePublicCodeWith[headerDetails](hostNS, "header_denied", apperr.User, "Denied.")

func TestHTTPBeforeError(t *testing.T) {
	cause := errors.New("private diagnosis")
	denied := apperr.EncapsulateWith(fmt.Errorf("wrapped: %w", cause), headerDenied, "Denied.", headerDetails{Reason: "limited"})
	for _, phase := range []string{"request", "guard", "admit", "authorize", "handle", "no hook", "unclassified", "success"} {
		t.Run(phase, func(t *testing.T) {
			h := adapter()
			h.Codes = httpkit.Codes{Namespaces: []apperr.Namespace{hostNS}, Status: map[apperr.Code]int{headerDenied.Code(): http.StatusForbidden}}
			err := denied
			if phase == "unclassified" {
				err = fmt.Errorf("private wrapper: %w", denied)
			}
			calls := 0
			h.BeforeError = func(w http.ResponseWriter, r *http.Request, got error) {
				calls++
				if !errors.Is(got, cause) {
					t.Errorf("original cause lost: %v", got)
				}
				if details, ok := apperr.DetailsOf(got, headerDenied); !ok || details.Reason != "limited" {
					t.Errorf("typed details lost: %+v, %v", details, ok)
				}
				if r.Header.Get("User") != "ann" {
					t.Errorf("request lost: %v", r.Header)
				}
				w.Header().Set("X-Error-Reason", "limited")
			}
			if phase == "no hook" {
				h.BeforeError = nil
			}
			if phase == "request" {
				h.Request = func(http.ResponseWriter, *http.Request) (request, error) { return request{}, err }
			}
			if phase == "guard" {
				h.Guard = func(apikit.Meta) func(http.ResponseWriter, *http.Request) error {
					return func(http.ResponseWriter, *http.Request) error { return err }
				}
			}
			a := admission
			if phase == "admit" {
				a.Admit = func(context.Context, request) (string, error) { return "", err }
			}
			if phase == "authorize" {
				a.Authorize = func(context.Context, request, string, any) error { return err }
			}
			d := greet("greetings.headers")
			d.Errors = []apperr.AnyCode{headerDenied}
			if phase == "handle" || phase == "no hook" || phase == "unclassified" {
				d.Handle = func(context.Context, string, greetIn) (greetOut, error) { return greetOut{}, err }
			}
			reg := newRegistry()
			apikit.Register(reg, a, d)
			w := do(httpkit.Wrap(h.Handler(runtime(), reg), httpkit.Config{Codes: h.Codes}), http.MethodPost, "/api/greetings.headers", "ann", `{"name":"Zed"}`)
			wantStatus, wantCalls, wantHeader := http.StatusForbidden, 1, "limited"
			if phase == "no hook" {
				wantCalls, wantHeader = 0, ""
			}
			if phase == "success" {
				wantStatus, wantCalls, wantHeader = http.StatusOK, 0, ""
			}
			if phase == "unclassified" {
				wantStatus = http.StatusInternalServerError
			}
			if w.Code != wantStatus || calls != wantCalls || w.Result().Header.Get("X-Error-Reason") != wantHeader {
				t.Fatalf("status=%d calls=%d headers=%v body=%s", w.Code, calls, w.Result().Header, w.Body)
			}
			if phase != "success" && phase != "unclassified" && !strings.Contains(w.Body.String(), `"details":{"reason":"limited"}`) {
				t.Errorf("shared envelope lost typed details: %s", w.Body)
			}
			if strings.Contains(w.Body.String(), "private") {
				t.Errorf("private cause leaked: %s", w.Body)
			}
		})
	}
}
