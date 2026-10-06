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

	"github.com/mockzilla/mockzilla-codegen/examples/server/gin/split/api"
	"github.com/mockzilla/mockzilla-codegen/examples/server/gin/split/books"
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

	handler := api.NewRouter(books.NewBooks(), api.WithMiddleware(
		books.RequestIDMiddleware,
		books.RecoverMiddleware,
		books.LoggingMiddleware,
		books.CORSMiddleware,
		books.TimeoutMiddleware(30*time.Second),
	))
	srv := &http.Server{
		Addr:              ":8080",
		Handler:           handler,
		ReadHeaderTimeout: 30 * time.Second,
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
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
