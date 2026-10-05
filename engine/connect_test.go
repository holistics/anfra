package engine_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"

	"github.com/holistics/anfra/engine"
)

// A host's transport wrapper carries every call to a ready sidecar, labelled by
// the sidecar, and not the readiness polling before it.
func TestWithTransportWrapsSidecarCalls(t *testing.T) {
	sidecarStub := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/rpc" {
				_, _ = w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}))
	}
	node, canal := sidecarStub(), sidecarStub()
	defer node.Close()
	defer canal.Close()

	var mu sync.Mutex
	var calls []string // "<sidecar> <path>"
	wrap := func(sidecar string, rt http.RoundTripper) http.RoundTripper {
		if rt == nil {
			t.Errorf("%s: wrapped a nil transport", sidecar)
		}
		return roundTripper(func(r *http.Request) (*http.Response, error) {
			mu.Lock()
			calls = append(calls, sidecar+" "+r.URL.Path)
			mu.Unlock()
			return rt.RoundTrip(r)
		})
	}

	ctx := context.Background()
	clients, closer, err := engine.Connect(ctx, node.URL, canal.URL, engine.WithTransport(wrap))
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	if len(calls) != 0 {
		t.Errorf("readiness polling went through the wrapper: %v", calls)
	}

	if _, err := clients.Node.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if err := clients.CanalQuery.Health(ctx); err != nil {
		t.Fatal(err)
	}
	want := []string{"anfra-node /rpc", "canal-query /health"}
	if !slices.Equal(calls, want) {
		t.Errorf("wrapped calls = %v, want %v", calls, want)
	}
}

// Without the option, Connect is as before.
func TestConnectWithoutOptions(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer stub.Close()
	_, closer, err := engine.Connect(context.Background(), stub.URL, stub.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = closer.Close()
}

type roundTripper func(*http.Request) (*http.Response, error)

func (f roundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
