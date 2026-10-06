package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/holistics/anfra/internal/app"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar"
	"github.com/holistics/anfra/internal/validate"
	"github.com/holistics/anfra/shared/apikit"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/httpkit"
	"github.com/spf13/cobra"
)

// defaultAddr is where `anfra serve` listens unless told otherwise, when it is
// free: a stable URL for apps and scripts. When another repo's server holds it,
// the OS picks a port, and the runtime file says which.
const defaultAddr = "127.0.0.1:7878"

func newServeCmd() *cobra.Command {
	var addr string
	var withMCP bool
	var idle time.Duration
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the core API over HTTP, keeping the sidecars warm for it and for CLI calls in this repo",
		Long: "Serve the core API over HTTP: every command as POST /api/core.<command>, its OpenAPI at\n" +
			"/api/openapi.json, the operations to discover at /api/ops, and /health. With --mcp, also the\n" +
			"same operations as MCP tools at /mcp. CLI calls in this repo use the server while it runs,\n" +
			"found through the repo's runtime file, so they skip starting the sidecars.\n\n" +
			"It listens on " + defaultAddr + ", or on a free port when another repo's server holds that one;\n" +
			"`anfra status` says where. It has no authentication: keep it on a loopback address.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd.Context(), addr, withMCP, idle)
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "", "the address to listen on, host:port (default "+defaultAddr+", or a free port)")
	cmd.Flags().BoolVar(&withMCP, "mcp", false, "also serve the operations as MCP tools, at /mcp (streamable HTTP)")
	// For a server someone else started (serve_daemon.md); a foreground one never times out.
	cmd.Flags().DurationVar(&idle, "idle-timeout", 0, "stop after this long without a request; 0 never")
	_ = cmd.Flags().MarkHidden("idle-timeout")
	return cmd
}

func runServe(ctx context.Context, addr string, withMCP bool, idle time.Duration) error {
	return withRepo(ctx, func(ctx context.Context, h hostContext) error {
		if f, ok := findServer(ctx, h.repo); ok {
			return fmt.Errorf("anfra serve is already running for this repo, at %s", f.URL)
		}
		ln, err := listen(h.repo, addr)
		if err != nil {
			return err
		}
		defer ln.Close()
		if host, _, _ := net.SplitHostPort(ln.Addr().String()); !isLoopback(host) {
			h.cfg.Logger.Warn("serve.exposed", "addr", ln.Addr().String(),
				"hint", "anfra serve has no authentication: anyone who reaches this address can query your data sources")
			fmt.Fprintf(os.Stderr, "warning: %s is not a loopback address; anfra serve has no authentication\n", ln.Addr())
		}

		// Warm sidecars live for the server's lifetime, so enable canal-query
		// connection pooling: DB connections are reused across requests.
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

		info := app.ServerInfo{URL: "http://" + ln.Addr().String(), InstanceID: newInstanceID(), Version: meta.Version}
		cc := h.commandContext(app.Clients{Node: node.Client(), CanalQuery: canal.Client()})
		cc.Server = &info

		ctx, stop := context.WithCancel(ctx)
		defer stop()
		handler := serveHandler(h.cfg.Logger, h.repo, cc, ln.Addr(), withMCP)
		if idle > 0 {
			handler = stopWhenIdle(ctx, handler, idle, stop)
		}
		srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}

		if err := writeRuntime(h.repo, runtimeFile{
			PID: os.Getpid(), URL: info.URL, RepoID: h.repo.ID, RepoDir: h.repo.Dir, InstanceID: info.InstanceID,
			Version: info.Version, StartedAt: time.Now().UTC(),
			Config: runtimeConfig{NodeURL: cfg.NodeURL, CanalQueryURL: cfg.CanalQueryURL},
		}); err != nil {
			return fmt.Errorf("record the server in %s: %w", runtimePath(h.repo), err)
		}
		defer removeRuntime(h.repo, info.InstanceID)

		go func() {
			<-ctx.Done() // SIGINT/SIGTERM, from the root context in main, or the idle timeout
			// Drain with a fresh deadline: WithoutCancel keeps ctx's values but drops
			// its (already-fired) cancellation, so Shutdown gets the full 5s.
			shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = srv.Shutdown(shutdownCtx)
		}()

		h.cfg.Logger.Info("serve.listening", "url", info.URL)
		fmt.Printf("anfra serve listening on %s (Ctrl-C to stop)\n", info.URL)
		if withMCP {
			fmt.Printf("MCP at %s/mcp\n", info.URL)
		}
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
}

// listen opens the server's listener: addr when given — refusing a taken one,
// and naming the repo whose server holds it — and otherwise defaultAddr, or a
// free loopback port when that one is taken.
func listen(r repo.Repo, addr string) (net.Listener, error) {
	explicit := addr != ""
	if !explicit {
		addr = defaultAddr
	}
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		return ln, nil
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	if explicit {
		if holder := holderOf(r, addr); holder != "" {
			return nil, fmt.Errorf("%s is taken by anfra serve for %s; pick another with --addr", addr, holder)
		}
		return nil, fmt.Errorf("%s is taken; pick another with --addr", addr)
	}
	return net.Listen("tcp", "127.0.0.1:0")
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// serveHandler is the whole HTTP surface: /health, the core API under /api with
// its OpenAPI and discovery, and with withMCP the same ops as MCP tools at /mcp —
// behind the request id, root span, log line and recovery, and the guards that
// keep a web page from using it (guard).
func serveHandler(logger *slog.Logger, r repo.Repo, cc app.CommandContext, addr net.Addr, withMCP bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		id := ""
		if cc.Server != nil {
			id = cc.Server.InstanceID
		}
		httpkit.WriteJSON(w, http.StatusOK, health{Status: "ok", RepoID: r.ID, InstanceID: id, Version: meta.Version})
	})
	rt, reg := app.NewRuntime(), app.NewRegistry()
	mux.Handle("/api/", coreAPI(cc).Handler(rt, reg))
	if withMCP {
		mux.Handle("/mcp", apikit.MCP[app.CommandContext]{
			Name: "anfra", Version: meta.Version, Codes: codes,
			Request: func(*http.Request) (app.CommandContext, error) { return cc, nil },
		}.Handler(rt, reg))
	}
	return httpkit.Wrap(guard(mux, addr), httpkit.Config{Logger: logger, Codes: codes})
}

// coreAPI is the core API over HTTP, every op run with cc: the host's one
// CommandContext — this repo, the warm sidecars, no data restrictions.
func coreAPI(cc app.CommandContext) apikit.HTTP[app.CommandContext] {
	return apikit.HTTP[app.CommandContext]{
		Codes: codes,
		Title: "anfra",
		// The contract's version, not the binary's: the committed document must
		// not change with every release.
		Version: "0",
		Request: func(http.ResponseWriter, *http.Request) (app.CommandContext, error) { return cc, nil },
	}
}

// codes are what `anfra serve` answers with: the engine's codes, besides
// apperr's generic ones, and the HTTP status of each.
var codes = httpkit.Codes{
	Namespaces: []apperr.Namespace{errcode.NS},
	Status: map[apperr.Code]int{
		validate.QueryInvalid.Code(): http.StatusUnprocessableEntity,
		// A data source failing to run a query is an upstream's failure.
		errcode.QueryFailed:        http.StatusBadGateway,
		errcode.SidecarUnavailable: http.StatusServiceUnavailable,
		// Unreachable here, since this server states its own context; kept so
		// every code has a status.
		errcode.UnknownCommand:         http.StatusNotFound,
		errcode.DataPermsMissing:       http.StatusInternalServerError,
		errcode.DataPermsUnenforceable: http.StatusInternalServerError,
	},
}

// guard refuses what a web page could make a browser send to this server:
//
//   - a Host other than the address it listens on or a loopback name, which is
//     DNS rebinding — a page whose host resolves to 127.0.0.1, reading answers
//     as if same-origin. Unchecked when it listens on every interface, where any
//     host name may reach it; that is the operator's warned choice.
//   - a cross-origin request that changes anything (http.CrossOriginProtection),
//     or one whose body is not JSON — a form can only send what needs no
//     preflight, and this server grants none. An MCP session's DELETE, which
//     ends it, has no body.
func guard(next http.Handler, addr net.Addr) http.Handler {
	host, _, _ := net.SplitHostPort(addr.String())
	anyHost := net.ParseIP(host) != nil && net.ParseIP(host).IsUnspecified()
	cop := http.NewCrossOriginProtection()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !anyHost {
			h, _, err := net.SplitHostPort(r.Host)
			if err != nil {
				h = r.Host
			}
			if h != host && !isLoopback(h) {
				httpkit.WriteError(w, r, apperr.New(apperr.InvalidRequest, "Unknown host "+r.Host+"."))
				return
			}
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if err := cop.Check(r); err != nil {
				httpkit.WriteError(w, r, apperr.Encapsulate(err, apperr.InvalidRequest, "Cross-origin request refused."))
				return
			}
			mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if r.Method != http.MethodDelete && (err != nil || mt != "application/json") {
				httpkit.WriteError(w, r, apperr.New(apperr.InvalidRequest, "Requests must be application/json."))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// stopWhenIdle calls stop once no request has started or been in flight for
// idle.
func stopWhenIdle(ctx context.Context, next http.Handler, idle time.Duration, stop func()) http.Handler {
	var inFlight atomic.Int64
	var last atomic.Int64
	last.Store(time.Now().UnixNano())
	go func() {
		t := time.NewTicker(min(idle/4, time.Minute))
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				if inFlight.Load() == 0 && now.Sub(time.Unix(0, last.Load())) >= idle {
					stop()
					return
				}
			}
		}
	}()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inFlight.Add(1)
		defer func() {
			last.Store(time.Now().UnixNano())
			inFlight.Add(-1)
		}()
		next.ServeHTTP(w, r)
	})
}

// newOpenAPICmd prints the core API's OpenAPI document, built from the
// registry with no repo or sidecars: for a client generated without a running
// server, and the committed api/openapi.yaml (scripts/openapi.sh).
func newOpenAPICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "openapi",
		Short: "Print the core API's OpenAPI document (the contract `anfra serve` serves)",
		Long: "Print the core API's OpenAPI document, in YAML: the contract `anfra serve` serves at\n" +
			"/api/openapi.json, here without a repo or a running server — to generate a client, or to\n" +
			"give an agent the API.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			spec, err := coreAPI(app.CommandContext{}).Spec(app.NewRuntime(), app.NewRegistry())
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(spec)
			return err
		},
	}
}

// --- the client: a CLI call routed to the repo's running server ---

// callServe runs a command on the server at url, and returns its answer's body.
// An error answer is returned as a remoteError, carrying the server's body.
func callServe(ctx context.Context, url, command string, input []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/api/"+app.OpName(command), bytes.NewReader(input))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call anfra serve at %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read anfra serve's answer: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		var env apperr.Envelope
		if json.Unmarshal(body, &env) == nil && env.Error.Code != "" {
			return nil, &remoteError{resp: env.Error}
		}
		return nil, fmt.Errorf("anfra serve answered %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
