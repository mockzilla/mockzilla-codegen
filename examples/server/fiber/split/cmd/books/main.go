// Written once by mockzilla-codegen. Edit it freely: generate does not overwrite it.

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	fiber "github.com/gofiber/fiber/v3"
	"github.com/mockzilla/mockzilla-codegen/examples/server/fiber/split/api"
	"github.com/mockzilla/mockzilla-codegen/examples/server/fiber/split/books"
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

	app := api.NewRouter(books.NewBooks(), api.WithConfig(fiber.Config{ReadTimeout: 30 * time.Second}), api.WithMiddleware(
		books.RequestIDMiddleware,
		books.RecoverMiddleware,
		books.LoggingMiddleware,
		books.CORSMiddleware,
		books.TimeoutMiddleware(30*time.Second),
	))

	errs := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", ":8080")
		errs <- app.Listen(":8080", fiber.ListenConfig{DisableStartupMessage: true})
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
	}
	return app.ShutdownWithTimeout(30 * time.Second)
}
