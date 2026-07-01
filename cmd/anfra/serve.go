package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/anfra-ai/anfra/internal/app"
	"github.com/anfra-ai/anfra/internal/repo"
	"github.com/anfra-ai/anfra/internal/sidecar"
	"github.com/spf13/cobra"
)

// serveSocketPath is the per-repo UDS the server listens on and CLI calls
// dial. Kept in the temp dir (short path, UDS sun_path is ~108 bytes) and keyed
// by repo ID so each repo has its own warm server.
func serveSocketPath(repo repo.Repo) string {
	return filepath.Join(os.TempDir(), "anfra-serve-"+repo.ID+".sock")
}

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the anfra server: keep sidecars warm and expose POST /call for agents and subsequent CLI calls",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runServe()
		},
	}
}

func runServe() error {
	return withRepo(func(h hostContext) error {
		if isServeRunning(h.repo) {
			return fmt.Errorf("anfra serve already running for this repo (socket %s)", serveSocketPath(h.repo))
		}

		// Warm sidecars live for the server's lifetime, so enable canal-query
		// connection pooling — DB connections are reused across /call requests.
		cfg := h.cfg
		cfg.EnablePooling = true

		node := sidecar.NewAnfraNode(cfg)
		if err := node.Start(h.ctx); err != nil {
			return fmt.Errorf("start anfra-node sidecar: %w", err)
		}
		defer node.Close()
		canal := sidecar.NewCanalQuery(cfg)
		if err := canal.Start(h.ctx); err != nil {
			return fmt.Errorf("start canal-query sidecar: %w", err)
		}
		defer canal.Close()

		clients := app.Clients{Node: node.Client(), Canal: canal.Client()}

		sockPath := serveSocketPath(h.repo)
		_ = os.Remove(sockPath)
		ln, err := net.Listen("unix", sockPath)
		if err != nil {
			return fmt.Errorf("listen on %s: %w", sockPath, err)
		}
		defer os.Remove(sockPath)

		srv := &http.Server{Handler: serveMux(h, clients)}

		go func() {
			sig := make(chan os.Signal, 1)
			signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
			<-sig
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(ctx)
		}()

		h.cfg.Logger.Info("serve.listening", "socket", sockPath)
		fmt.Printf("anfra serve listening on %s (Ctrl-C to stop)\n", sockPath)
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	})
}

func serveMux(h hostContext, clients app.Clients) http.Handler {
	// Requests run concurrently: anfra-node compiles per-request (fresh Program
	// from the serialized cache, no shared in-memory program) and canal-query is
	// built for concurrency, so no serialization is needed. (A future warm
	// in-memory program cache would guard itself inside anfra-node.)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})
	mux.HandleFunc("/call", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeCallError(w, http.StatusMethodNotAllowed, "use POST")
			return
		}
		var req app.Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeCallError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}

		// `help` (truthy) → help text from the registry, mirroring `anfra <cmd> --help`.
		if app.IsTruthy(req.Args["help"]) {
			text, err := app.Help(req.Command)
			if err != nil {
				writeCallError(w, http.StatusNotFound, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"help": text})
			return
		}

		res, err := app.Dispatch(r.Context(), clients, h.repo, req)
		if err != nil {
			writeCallError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeCallError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

// --- serve client (used by one-shot CLI calls to reach a warm server) ---

func serveHTTPClient(repo repo.Repo) *http.Client {
	sockPath := serveSocketPath(repo)
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", sockPath)
			},
		},
	}
}

// isServeRunning reports whether a warm server is reachable for this repo.
func isServeRunning(repo repo.Repo) bool {
	conn, err := net.DialTimeout("unix", serveSocketPath(repo), 200*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// callServe POSTs the request to the warm server and returns the response body
// plus its Content-Type, surfacing a structured {error} as a Go error.
func callServe(repo repo.Repo, req app.Request) (body []byte, contentType string, err error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, "", fmt.Errorf("marshal request: %w", err)
	}
	httpReq, err := http.NewRequest(http.MethodPost, "http://unix/call", bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := serveHTTPClient(repo).Do(httpReq)
	if err != nil {
		return nil, "", fmt.Errorf("call serve: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read serve response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			return nil, "", fmt.Errorf("%s", e.Error)
		}
		return nil, "", fmt.Errorf("serve returned status %d", resp.StatusCode)
	}
	return data, resp.Header.Get("Content-Type"), nil
}
