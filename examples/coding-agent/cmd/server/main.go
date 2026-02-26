package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/app"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/config"
)

func main() {
	logger := newServerLogger(serverLogOutput)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load config", slog.Any("error", err))
		os.Exit(1)
	}

	application, err := app.New(cfg, logger)
	if err != nil {
		logger.Error("initialize app", slog.Any("error", err))
		os.Exit(1)
	}

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- application.Start()
	}()
	logger.Info("server started", slog.String("addr", cfg.HTTPAddr))

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErrCh:
		if err != nil {
			logger.Error("server exited", slog.Any("error", err))
			os.Exit(1)
		}
		return
	case <-sigCtx.Done():
	}
	logger.Info("shutdown initiated")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := application.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown server", slog.Any("error", err))
		os.Exit(1)
	}

	if err := <-serverErrCh; err != nil {
		logger.Error("server stopped with error", slog.Any("error", err))
		os.Exit(1)
	}

	logger.Info("server stopped")
}
