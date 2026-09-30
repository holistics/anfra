package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/holistics/anfra/internal/app"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/spf13/cobra"
)

// serveSocketPath is the per-repo UDS the server listens on and CLI calls
// dial. Kept in the temp dir (short path, UDS sun_path is ~108 bytes) and keyed
// by repo ID so each repo has its own warm server.
func serveSocketPath(repo repo.Repo) string {
	return filepath.Join(os.TempDir(), "anfra-serve-"+repo.ID+".sock")
}

func newServeCmd() *cobra.Command {
	var httpAddr string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the anfra server: keep sidecars warm and expose POST /call for agents and subsequent CLI calls",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd.Context(), httpAddr)
		},
	}
	cmd.Flags().StringVar(&httpAddr, "http", "",
		"also serve over TCP at this address: <port>, :<port> or <host>:<port> (host defaults to 127.0.0.1; :0 picks a free port)")
	return cmd
}

func runServe(ctx context.Context, httpAddr string) error {
	return withRepo(ctx, func(ctx context.Context, h hostContext) error {
		if isServeRunning(h.repo) {
			return fmt.Errorf("anfra serve already running for this repo (socket %s)", serveSocketPath(h.repo))
		}

		// Bind the optional TCP listener before the (slow) sidecar startup, so a
		// taken port fails fast instead of after the sidecars are warm.
		var tcpLn net.Listener
		if httpAddr != "" {
			ln, err := listenHTTP(httpAddr)
			if err != nil {
				return err
			}
			defer ln.Close()
			tcpLn = ln
			if host, _, _ := net.SplitHostPort(ln.Addr().String()); !isLoopbackHost(host) {
				msg := fmt.Sprintf("HTTP listener %s is reachable from other machines; /call has no authentication and runs queries with your data source credentials", ln.Addr())
				fmt.Fprintln(os.Stderr, "WARNING:", msg)
				h.cfg.Logger.Warn("serve.http_exposed", "addr", ln.Addr().String())
			}
		}

		// Warm sidecars live for the server's lifetime, so enable canal-query
		// connection pooling — DB connections are reused across /call requests.
		cfg := h.cfg
		cfg.EnablePooling = true

		node := sidecar.NewAnfraNode(cfg)
		if err := node.Start(ctx); err != nil {
			return fmt.Errorf("start anfra-node sidecar: %w", err)
		}
		defer node.Close()
		canal := sidecar.NewCanalQuery(cfg)
		if err := canal.Start(ctx); err != nil {
			return fmt.Errorf("start canal-query sidecar: %w", err)
		}
		defer canal.Close()

		clients := app.Clients{Node: node.Client(), CanalQuery: canal.Client()}
		if tcpLn != nil {
			clients.HTTPAddr = tcpLn.Addr().String()
		}

		sockPath := serveSocketPath(h.repo)
		_ = os.Remove(sockPath)
		ln, err := net.Listen("unix", sockPath)
		if err != nil {
			return fmt.Errorf("listen on %s: %w", sockPath, err)
		}
		defer os.Remove(sockPath)

		mux := serveMux(h, clients)
		servers := []*http.Server{{Handler: mux, ReadHeaderTimeout: 10 * time.Second}}
		listeners := []net.Listener{ln}
		if tcpLn != nil {
			// The TCP listener is reachable by browsers, so it gets the browser
			// defenses; the socket keeps serving the bare mux.
			servers = append(servers, &http.Server{Handler: httpGuard(tcpLn.Addr(), mux), ReadHeaderTimeout: 10 * time.Second})
			listeners = append(listeners, tcpLn)
		}

		errCh := make(chan error, len(servers))
		for i, srv := range servers {
			go func() {
				if err := srv.Serve(listeners[i]); err != nil && err != http.ErrServerClosed {
					errCh <- err
				}
			}()
		}

		h.cfg.Logger.Info("serve.listening", "socket", sockPath, "http", clients.HTTPAddr)
		fmt.Printf("anfra serve listening on %s (Ctrl-C to stop)\n", sockPath)
		if tcpLn != nil {
			fmt.Printf("anfra serve listening on http://%s\n", tcpLn.Addr())
		}

		var serveErr error
		select {
		case <-ctx.Done(): // cancelled on SIGINT/SIGTERM by the root context in main
		case serveErr = <-errCh: // a listener failed; stop the other one too
		}
		// Drain with a fresh deadline: WithoutCancel keeps ctx's values but drops
		// its (already-fired) cancellation, so Shutdown gets the full 5s.
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		for _, srv := range servers {
			_ = srv.Shutdown(shutdownCtx)
		}
		return serveErr
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

		// `help` (truthy) → the command's cobra help text, identical to `anfra <cmd> --help`.
		if app.IsTruthy(req.Args["help"]) {
			// commandHelp only builds the command tree to render help text; it never
			// executes a RunE, so there's no request context to thread into a command
			// body. False positive — the rest of the tree is properly ctx-threaded.
			text, err := commandHelp(req.Command) //nolint:contextcheck
			if err != nil {
				writeCallError(w, http.StatusNotFound, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, app.Response{Status: app.StatusOK, Data: map[string]any{"help": text}})
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

// listenHTTP binds the --http TCP listener. A bare port or an empty host means
// 127.0.0.1, so the short forms never expose the server beyond this machine.
func listenHTTP(addr string) (net.Listener, error) {
	norm, err := normalizeHTTPAddr(addr)
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", norm)
	if err != nil {
		return nil, fmt.Errorf("listen on --http %s: %w", norm, err)
	}
	return ln, nil
}

func normalizeHTTPAddr(addr string) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr != "" && strings.Trim(addr, "0123456789") == "" {
		addr = ":" + addr // bare port
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "", fmt.Errorf("invalid --http address %q: want <port>, :<port> or <host>:<port>", addr)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port), nil
}

// isLoopbackHost reports whether host (a hostname or IP, without port) names
// this machine's loopback interface.
func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// httpGuard wraps the TCP listener's handler with browser defenses the socket
// doesn't need: any web page can send simple cross-origin requests to a loopback
// port, and DNS rebinding can point a foreign hostname at it.
//   - POST /call must be application/json. That is never a CORS "simple" request,
//     so a browser must preflight it — and the preflight fails (no CORS headers).
//   - Host must name the bound address (loopback names are always accepted). A
//     wildcard bind (0.0.0.0, ::) accepts any Host, since it was opted into.
func httpGuard(bound net.Addr, next http.Handler) http.Handler {
	boundHost, _, _ := net.SplitHostPort(bound.String())
	boundIP := net.ParseIP(boundHost)
	checkHost := boundIP == nil || !boundIP.IsUnspecified()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if checkHost {
			host := r.Host
			if hp, _, err := net.SplitHostPort(host); err == nil {
				host = hp
			}
			host = strings.Trim(host, "[]")
			if !isLoopbackHost(host) && !strings.EqualFold(host, boundHost) {
				writeCallError(w, http.StatusForbidden, fmt.Sprintf("host %q not allowed", r.Host))
				return
			}
		}
		if r.Method == http.MethodPost && r.URL.Path == "/call" {
			if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
				writeCallError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// commandHelp returns cobra's help text for a command (empty name = the root),
// so /call help is identical to the CLI's `--help` — one help renderer, not two.
func commandHelp(name string) (string, error) {
	root := newRootCmd()
	target := root
	if name != "" {
		found, _, err := root.Find([]string{name})
		if err != nil || found == root {
			return "", fmt.Errorf(`unknown command %q; send {"help": true} to list commands`, name)
		}
		target = found
	}
	var buf bytes.Buffer
	target.SetOut(&buf)
	if err := target.Help(); err != nil {
		return "", err
	}
	return buf.String(), nil
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
