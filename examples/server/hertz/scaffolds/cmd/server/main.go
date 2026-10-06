// Written once by mockzilla-codegen. Edit it freely: generate does not overwrite it.

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/mockzilla/mockzilla-codegen/examples/server/hertz/scaffolds"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	h := scaffolds.NewRouter(scaffolds.NewTodo(), scaffolds.WithConfig(server.WithHostPorts(":9000"), server.WithReadTimeout(5*time.Second)), scaffolds.WithMiddleware(
		scaffolds.RequestIDMiddleware,
		scaffolds.RecoverMiddleware,
		scaffolds.LoggingMiddleware,
		scaffolds.CORSMiddleware,
		scaffolds.TimeoutMiddleware(5*time.Second),
	))

	errs := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", ":9000")
		errs <- h.Run()
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return h.Shutdown(shutdownCtx)
}
