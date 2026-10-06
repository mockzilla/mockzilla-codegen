// Written once by mockzilla-codegen. Edit it freely: generate does not overwrite it.

package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mockzilla/mockzilla-codegen/examples/server/iris/scaffolds"
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

	handler := scaffolds.NewRouter(scaffolds.NewTodo(), scaffolds.WithMiddleware(
		scaffolds.RequestIDMiddleware,
		scaffolds.RecoverMiddleware,
		scaffolds.LoggingMiddleware,
		scaffolds.CORSMiddleware,
		scaffolds.TimeoutMiddleware(5*time.Second),
	))
	srv := &http.Server{
		Addr:              ":9000",
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", srv.Addr)
		errs <- srv.ListenAndServe()
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
