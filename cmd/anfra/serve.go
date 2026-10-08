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
	"github.com/holistics/anfra/internal/appserve"
	"github.com/holistics/anfra/internal/command"
	"github.com/holistics/anfra/internal/errcode"
	"github.com/holistics/anfra/internal/meta"
	"github.com/holistics/anfra/internal/query"
	"github.com/holistics/anfra/internal/repo"
	"github.com/holistics/anfra/internal/sidecar/anfranode"
	"github.com/holistics/anfra/internal/sidecar/canalquery"
	"github.com/holistics/anfra/shared/apikit"
	"github.com/holistics/anfra/shared/apperr"
	"github.com/holistics/anfra/shared/httpkit"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// defaultAddr is where `anfra serve` listens unless told otherwise, when it is
// free: a stable URL for apps and scripts. When another repo's server holds it,
// the OS picks a port, and the runtime file says which.
const defaultAddr = "127.0.0.1:7878"

// serveOptions are anfra serve's flags. Everything is served unless turned off.
type serveOptions struct {
	addr  string
	noMCP bool
	// noApps leaves out the repo's Data Apps, at /apps/; noWatch, their live reload.
	noApps  bool
	noWatch bool
	idle    time.Duration
}

func newServeCmd() *cobra.Command {
	var opts serveOptions
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the repo over HTTP: the core API, MCP and the Data Apps, with the sidecars kept warm",
		Long: "Serve the repo over HTTP:\n\n" +
			"  /api/core.<command>  every command, as POST; its OpenAPI at /api/openapi.json, the\n" +
			"                       operations to discover at /api/ops\n" +
			"  /mcp                 the same operations as MCP tools (streamable HTTP)\n" +
			"  /apps/<path>         the repo's Data Apps (apps/**.html) in a browser, live-reloading as\n" +
			"                       their files or the AML change\n" +
			"  /health\n\n" +
			"CLI calls in this repo use the server while it runs, found through the repo's runtime file,\n" +
			"so they skip starting the sidecars.\n\n" +
			"It listens on " + defaultAddr + ", or on a free port when another repo's server holds that one;\n" +
			"`anfra status` says where. It has no authentication: keep it on a loopback address.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServe(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.addr, "addr", "", "the address to listen on, host:port (default "+defaultAddr+", or a free port)")
	cmd.Flags().BoolVar(&opts.noMCP, "no-mcp", false, "don't serve the operations as MCP tools")
	cmd.Flags().BoolVar(&opts.noApps, "no-apps", false, "don't serve the repo's Data Apps")
	cmd.Flags().BoolVar(&opts.noWatch, "no-watch", false, "serve the Data Apps without live reload, e.g. to host them for others")
	// For a server someone else started (serve_daemon.md); a foreground one never times out.
	cmd.Flags().DurationVar(&opts.idle, "idle-timeout", 0, "stop after this long without a request; 0 never")
	_ = cmd.Flags().MarkHidden("idle-timeout")
	return cmd
}

func runServe(ctx context.Context, opts serveOptions) error {
	return withRepo(ctx, func(ctx context.Context, h hostContext) error {
		if f, ok := findServer(ctx, h.repo); ok {
			return fmt.Errorf("anfra serve is already running for this repo, at %s", f.URL)
		}
		ln, err := listen(h.repo, opts.addr)
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
		node := anfranode.New(cfg)
		if err := node.Start(ctx); err != nil {
			return fmt.Errorf("start anfra-node sidecar: %w", err)
		}
		defer node.Close()
		canal := canalquery.New(cfg)
		if err := canal.Start(ctx); err != nil {
			return fmt.Errorf("start canal-query sidecar: %w", err)
		}
		defer canal.Close()

		info := command.ServerInfo{URL: "http://" + ln.Addr().String(), InstanceID: newInstanceID(), Version: meta.Version}
		cc := h.commandContext(command.Clients{Node: node.Client(), CanalQuery: canal.Client()})
		cc.Server = &info

		ctx, stop := context.WithCancel(ctx)
		defer stop()
		var apps *appserve.Server
		if !opts.noApps {
			apps = appserve.New(appserve.Options{
				RepoDir: h.repo.Dir, Watch: !opts.noWatch, Logger: h.cfg.Logger,
				// make dev's Vite dev server, which serves the frontend from source.
				DevFrontendURL: os.Getenv("ANFRA_APPSERVE_DEV_URL"),
			})
			if !apps.FrontendBuilt() {
				fmt.Fprintln(os.Stderr, "warning: this anfra was built without its Data App pages; /apps/ says how to build them")
			}
		}
		handler := serveHandler(h.cfg.Logger, h.repo, cc, ln.Addr(), !opts.noMCP, apps)
		if opts.idle > 0 {
			handler = stopWhenIdle(ctx, handler, opts.idle, stop)
		}
		srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
		if apps != nil {
			// Open event streams would hold Shutdown for its whole deadline.
			srv.RegisterOnShutdown(apps.Close)
		}

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
		if apps != nil {
			fmt.Printf("  Data Apps  %s/apps/\n", info.URL)
		}
		if !opts.noMCP {
			fmt.Printf("  MCP        %s/mcp\n", info.URL)
		}
		if !updateNotifyDisabled() {
			go watchUpdates(ctx, h.cfg.Logger)
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
// its OpenAPI and discovery, with withMCP the same ops as MCP tools at /mcp, and
// with apps the repo's Data Apps (/, /apps/, /appserve/) — behind the request id,
// root span, log line and recovery, and the guards that keep a web page from
// using it (guard).
func serveHandler(logger *slog.Logger, r repo.Repo, cc command.CommandContext, addr net.Addr, withMCP bool, apps *appserve.Server) http.Handler {
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
		mux.Handle("/mcp", apikit.MCP[command.CommandContext]{
			Name: "anfra", Version: meta.Version, Codes: codes,
			Request: func(*http.Request) (command.CommandContext, error) { return cc, nil },
		}.Handler(rt, reg))
	}
	if apps != nil {
		mux.Handle("/{$}", apps)
		mux.Handle("/apps", apps)
		mux.Handle("/apps/", apps)
		mux.Handle("/appserve/", apps)
	}
	// On loopback, a caller is this machine's user, whose CLI sends its trace along:
	// continue it. Exposed, a caller's trace is only linked.
	host, _, _ := net.SplitHostPort(addr.String())
	return httpkit.Wrap(guard(mux, addr), httpkit.Config{Logger: logger, Codes: codes, TrustTraceparent: isLoopback(host)})
}

// coreAPI is the core API over HTTP, every op run with cc: the host's one
// CommandContext — this repo, the warm sidecars, no data restrictions.
func coreAPI(cc command.CommandContext) apikit.HTTP[command.CommandContext] {
	return apikit.HTTP[command.CommandContext]{
		Codes: codes,
		Title: "anfra",
		// The contract's version, not the binary's: the committed document must
		// not change with every release.
		Version: "0",
		Request: func(http.ResponseWriter, *http.Request) (command.CommandContext, error) { return cc, nil },
	}
}

// codes are what `anfra serve` answers with: the engine's codes, besides
// apperr's generic ones, and the HTTP status of each.
var codes = httpkit.Codes{
	Namespaces: []apperr.Namespace{errcode.NS},
	Status: map[apperr.Code]int{
		query.QueryInvalid.Code(): http.StatusUnprocessableEntity,
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
			spec, err := coreAPI(command.CommandContext{}).Spec(app.NewRuntime(), app.NewRegistry())
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(spec)
			return err
		},
	}
}

// --- the client: a CLI call routed to the repo's running server ---

// serveClient calls the repo's running server, carrying the CLI's trace into it.
var serveClient = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport,
	otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
		return "anfra serve " + r.Method + " " + r.URL.Path
	}))}

// callServe runs a command on the server at url, and returns its answer's body.
// An error answer is returned as a remoteError, carrying the server's body.
func callServe(ctx context.Context, url, name string, input []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/api/"+command.OpName(name), bytes.NewReader(input))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := serveClient.Do(req)
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
