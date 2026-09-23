// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package scaffolds

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mockzilla/mockzilla-codegen/examples/server/goframe/internal/goframetest"
	"github.com/stretchr/testify/assert"
)

func TestServiceStub(t *testing.T) {
	t.Parallel()

	router := goframetest.Handler(t, NewRouter(NewTodo()))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, httptest.NewRequest("GET", "/todos", nil))

	assert.Equal(t, 500, rec.Code)
	assert.Equal(t, `{"error":"internal server error"}`, rec.Body.String())
}

func TestMiddleware(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{ReplaceAttr: dropTimeAndDuration})))
	router := goframetest.Handler(t, NewRouter(NewTodo(), WithMiddleware(RequestIDMiddleware, RecoverMiddleware, LoggingMiddleware, CORSMiddleware, TimeoutMiddleware(10*time.Millisecond))))

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
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/nope", nil))
	assert.Equal(t, 404, rec.Code)
	assert.NotEmpty(t, rec.Header().Get("X-Request-ID"), "the middleware wraps every path of a new router")

	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("nope") })
	slow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(time.Second):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
		}
	})

	rec = httptest.NewRecorder()
	RecoverMiddleware(panicking).ServeHTTP(rec, httptest.NewRequest("GET", "/panic", nil))
	assert.Equal(t, 500, rec.Code, "a panic is answered")
	assert.Contains(t, logs.String(), `level=ERROR msg=panic error=nope`)

	rec = httptest.NewRecorder()
	TimeoutMiddleware(10*time.Millisecond)(slow).ServeHTTP(rec, httptest.NewRequest("GET", "/slow", nil))
	assert.Equal(t, 503, rec.Code, "the timeout applies")
}

func dropTimeAndDuration(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey || a.Key == "duration" {
		return slog.Attr{}
	}
	return a
}
