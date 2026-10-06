// Package httpkit is what every HTTP server built on the anfra packages shares:
// the request id, the root span, the request log line, panic recovery, and the
// one renderer of an error, with the HTTP status of each code served. apikit's
// HTTP adapter renders through it, and so do servers that are not op APIs (the
// git service of anfra-cloud, say).
//
// It holds nothing a particular server decides: its catalog of codes and their
// statuses are passed in (Codes), and routing, credentials, CSRF and body limits
// stay with the server that needs them.
package httpkit

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"
)

// Config is what a server tells the shared middleware about itself.
type Config struct {
	Logger *slog.Logger // slog.Default() when nil
	// Codes are what the server serves: every error is rendered, and logged, as
	// its served code.
	Codes Codes
	// TrustTraceparent continues a caller's trace, for a server whose callers are
	// this deployment's own services. Otherwise the endpoint is public: a caller's
	// traceparent is kept as a link, and the request starts a trace of ours.
	TrustTraceparent bool
}

// Wrap puts the shared middleware in front of h, outermost first: request id,
// root span, request log line, recovery. A handler inside reports what the log
// line and the span cannot see for themselves through Routed and Canceled, and
// renders every error through WriteError.
func Wrap(h http.Handler, cfg Config) http.Handler {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	h = recoverPanics(h)
	h = logRequests(h, cfg.Logger)
	h = traceRequests(h, cfg.TrustTraceparent)
	h = assignRequestID(h)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), codesKey{}, cfg.Codes)))
	})
}

// Serve runs srv until ctx ends, then drains it, giving in-flight requests ten
// seconds. A listener that fails — the port taken, say — ends it too, with that
// error.
func Serve(ctx context.Context, srv *http.Server) error {
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	g.Go(func() error {
		<-gctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	})
	return g.Wait()
}
