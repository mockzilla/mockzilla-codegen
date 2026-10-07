// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package httpserver

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetachRequest(t *testing.T) {
	t.Parallel()

	buf := []byte("/pets/1?tag=a")
	uri := unsafe.String(&buf[0], len(buf))
	r := httptest.NewRequest(http.MethodGet, uri, nil)
	r.RequestURI = uri
	r.Header.Set("X-Tag", uri)

	got := DetachRequest(r)
	copy(buf, "/xxxx/9?tag=b")

	assert.Equal(t, "/pets/1?tag=a", got.RequestURI)
	assert.Equal(t, "/pets/1", got.URL.Path)
	assert.Equal(t, "a", got.URL.Query().Get("tag"))
	assert.Equal(t, "/pets/1?tag=a", got.Header.Get("X-Tag"))
	assert.Equal(t, "example.com", got.Host)
}

func TestDetachRequestBadURI(t *testing.T) {
	t.Parallel()

	r := httptest.NewRequest(http.MethodGet, "/pets", nil)
	r.RequestURI = "::"

	got := DetachRequest(r)

	assert.Equal(t, "/pets", got.URL.Path, "the URL is kept when the request URI does not parse")
}

func TestDetachPathValue(t *testing.T) {
	t.Parallel()

	buf := []byte("rex")
	raw := unsafe.String(&buf[0], len(buf))

	got := DetachPathValue(raw)
	copy(buf, "max")

	assert.Equal(t, "rex", got)
	assert.Equal(t, "john@example.com", DetachPathValue("john%40example.com"))
	assert.Equal(t, "a%zz", DetachPathValue("a%zz"))
}

func TestRecover(t *testing.T) {
	t.Parallel()

	var got error
	eh := ErrorHandlerFunc(func(w http.ResponseWriter, _ *http.Request, status int, err error) {
		got = err
		w.WriteHeader(status)
	})
	h := Recover(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }), eh)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	require.ErrorIs(t, got, ErrPanic)
	require.EqualError(t, errors.Unwrap(got), "panic: boom")
}

func TestRecoverWithoutPanic(t *testing.T) {
	t.Parallel()

	h := Recover(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }), DefaultErrorHandler{})
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusNoContent, rec.Code)
}
