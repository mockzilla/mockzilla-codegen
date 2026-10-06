// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package scaffolds

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestServiceStub(t *testing.T) {
	t.Parallel()

	router := NewRouter(NewTodo())
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, httptest.NewRequest("GET", "/todos", nil))

	assert.Equal(t, 500, rec.Code)
	assert.Equal(t, `{"error":"internal server error"}`, rec.Body.String())
}

func TestMiddleware(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{ReplaceAttr: dropTimeAndDuration})))
	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("nope") })
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			if errors.Is(r.Context().Err(), context.DeadlineExceeded) {
				w.WriteHeader(http.StatusServiceUnavailable)
			}
		}
	})
	router := NewRouter(NewTodo(), WithMiddleware(RequestIDMiddleware, RecoverMiddleware, LoggingMiddleware, CORSMiddleware, TimeoutMiddleware(10*time.Millisecond)))
	router.Get("/panic", panicking)
	router.Get("/slow", slow)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("OPTIONS", "/todos", nil))
	assert.Equal(t, 204, rec.Code, "CORS preflight")
	assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Len(t, rec.Header().Get("X-Request-ID"), 26, "a request id is made")

	req := httptest.NewRequest("GET", "/todos", nil)
	req.Header.Set("X-Request-ID", "given")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	assert.Equal(t, "given", rec.Header().Get("X-Request-ID"), "a request id is kept")
	assert.Contains(t, logs.String(), `level=INFO msg=request method=GET path=/todos status=500`)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/panic", nil))
	assert.Equal(t, 500, rec.Code, "a panic is answered")
	assert.NotEmpty(t, rec.Header().Get("X-Request-ID"), "the middleware wraps every route of a new router")
	assert.Contains(t, logs.String(), `level=ERROR msg=panic error=nope`)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/slow", nil))
	assert.Equal(t, 503, rec.Code, "the request's context ends after the timeout")
}

func dropTimeAndDuration(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey || a.Key == "duration" {
		return slog.Attr{}
	}
	return a
}
