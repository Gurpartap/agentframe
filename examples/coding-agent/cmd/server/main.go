package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/app"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	application, err := app.New(cfg)
	if err != nil {
		log.Fatalf("new app: %v", err)
	}

	serverErrCh := make(chan error, 1)
	go func() {
		serverErrCh <- application.Start()
	}()

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serverErrCh:
		if err != nil {
			log.Fatalf("server exited: %v", err)
		}
		return
	case <-sigCtx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := application.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("shutdown server: %v", err)
	}

	if err := <-serverErrCh; err != nil {
		log.Fatalf("server stopped with error: %v", err)
	}
}
