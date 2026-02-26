package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync/atomic"

	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/config"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/httpapi"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/runtimewire"
)

// App owns runtime wiring and HTTP server lifecycle.
type App struct {
	cfg               config.Config
	runtime           *runtimewire.Runtime
	server            *http.Server
	cancelServerScope context.CancelFunc
	ready             atomic.Bool
}

func New(cfg config.Config, logger *slog.Logger) (*App, error) {
	if cfg.HTTPAddr == "" {
		return nil, errors.New("new app: empty HTTPAddr")
	}
	if logger == nil {
		return nil, errors.New("new app: nil logger")
	}
	if cfg.ShutdownTimeout <= 0 {
		return nil, errors.New("new app: shutdown timeout must be > 0")
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("new app config: %w", err)
	}

	runtime, err := runtimewire.NewWithLogger(cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("new app runtime: %w", err)
	}

	serverScopeCtx, cancelServerScope := context.WithCancel(context.Background())
	a := &App{
		cfg:               cfg,
		runtime:           runtime,
		cancelServerScope: cancelServerScope,
	}

	apiRouter := httpapi.NewRouter(runtime)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", a.handleHealthz)
	mux.HandleFunc("/readyz", a.handleReadyz)
	mux.Handle("/", apiRouter)
	handler := requestLoggingMiddleware(logger)(mux)
	a.server = &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: handler,
		BaseContext: func(_ net.Listener) context.Context {
			return serverScopeCtx
		},
	}

	return a, nil
}

func (a *App) Start() error {
	a.ready.Store(true)

	err := a.server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	a.ready.Store(false)
	return err
}

func (a *App) Shutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("shutdown: nil context")
	}
	a.ready.Store(false)
	a.cancelServerScope()
	return a.server.Shutdown(ctx)
}

func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writePlain(w, http.StatusOK, "ok")
}

func (a *App) handleReadyz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !a.ready.Load() || a.runtime == nil {
		writePlain(w, http.StatusServiceUnavailable, "not ready")
		return
	}
	writePlain(w, http.StatusOK, "ready")
}

func writePlain(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}
