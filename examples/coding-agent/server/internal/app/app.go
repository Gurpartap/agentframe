package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"

	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/config"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/httpapi"
	"github.com/Gurpartap/agentframe/examples/coding-agent/server/internal/runtimewire"
)

// App owns runtime wiring and HTTP server lifecycle.
type App struct {
	cfg     config.Config
	logger  *slog.Logger
	runtime *runtimewire.Runtime
	server  *http.Server
	ready   atomic.Bool
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

	a := &App{
		cfg:     cfg,
		logger:  logger,
		runtime: runtime,
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

	err := a.server.Shutdown(ctx)
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		a.logger.Warn("graceful shutdown timed out; forcing connection close")
		if closeErr := a.server.Close(); closeErr != nil {
			return fmt.Errorf("shutdown timeout and forced close failed: %w", errors.Join(err, closeErr))
		}
		return nil
	}
	return err
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
