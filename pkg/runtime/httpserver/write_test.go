// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package httpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"iter"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

type note string

// brokenWriter counts its flushes and fails them from number flushFails on.
type brokenWriter struct {
	*httptest.ResponseRecorder
	isWriteBroken bool
	flushes       int
	flushFails    int
}

func (w *brokenWriter) Write(p []byte) (int, error) {
	if w.isWriteBroken {
		return 0, errors.New("gone")
	}
	return w.ResponseRecorder.Write(p)
}

func (w *brokenWriter) FlushError() error {
	w.flushes++
	if w.flushFails > 0 && w.flushes >= w.flushFails {
		return errors.New("gone")
	}
	return nil
}

func TestWrite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		headers         http.Header
		body            any
		wantContentType string
		wantBody        string
	}{
		{name: "No body", body: nil},
		{name: "JSON", body: map[string]int{"a": 1}, wantContentType: "application/json", wantBody: `{"a":1}`},
		{name: "JSON with the content type given", headers: http.Header{"Content-Type": {"application/problem+json"}}, body: 1, wantContentType: "application/problem+json", wantBody: "1"},
		{name: "Text", body: "hi", wantContentType: "application/octet-stream", wantBody: "hi"},
		{name: "Text behind a pointer", body: runtime.Ptr("hi"), wantContentType: "application/octet-stream", wantBody: "hi"},
		{name: "Bytes", headers: http.Header{"Content-Type": {"text/plain"}}, body: []byte("hi"), wantContentType: "text/plain", wantBody: "hi"},
		{name: "Bytes behind a pointer", body: runtime.Ptr([]byte("hi")), wantContentType: "application/octet-stream", wantBody: "hi"},
		{name: "File", body: runtime.NewFile([]byte("data"), "a.bin", "image/png"), wantContentType: "image/png", wantBody: "data"},
		{name: "File pointer", body: runtime.Ptr(runtime.NewFileReader(strings.NewReader("data"), "a", "", -1)), wantContentType: "", wantBody: "data"},
		{name: "A string under JSON", headers: http.Header{"Content-Type": {"application/json"}}, body: runtime.Ptr("hi"), wantContentType: "application/json", wantBody: `"hi"`},
		{name: "A file under JSON", headers: http.Header{"Content-Type": {"application/json"}}, body: runtime.Ptr(runtime.NewFile([]byte("hi"), "", "")), wantContentType: "application/json", wantBody: `"aGk="`},
		{name: "A nil file", headers: http.Header{"Content-Type": {"image/png"}}, body: (*runtime.File)(nil), wantContentType: "image/png"},
		{name: "JSON with parameters", headers: http.Header{"Content-Type": {"application/json; charset=utf-8"}}, body: map[string]int{"a": 1}, wantContentType: "application/json; charset=utf-8", wantBody: `{"a":1}`},
		{name: "A defined string as it is", headers: http.Header{"Content-Type": {"text/plain"}}, body: runtime.Ptr(note("hi")), wantContentType: "text/plain", wantBody: "hi"},
		{name: "A nil pointer without JSON", headers: http.Header{"Content-Type": {"application/xml"}}, body: (*struct{})(nil), wantContentType: "application/xml"},
		{name: "A form", headers: http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}, body: map[string]any{"text": "x y", "stars": 2}, wantContentType: "application/x-www-form-urlencoded", wantBody: "stars=2&text=x%20y"},
		{name: "A number as text", headers: http.Header{"Content-Type": {"text/plain"}}, body: runtime.Ptr(42), wantContentType: "text/plain", wantBody: "42"},
		{name: "A time as text", headers: http.Header{"Content-Type": {"text/plain"}}, body: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), wantContentType: "text/plain", wantBody: "2026-01-02T03:04:05Z"},
		{name: "Events", headers: http.Header{"Content-Type": {"text/event-stream"}}, body: slices.Values([]map[string]int{{"a": 1}, {"a": 2}}), wantContentType: "text/event-stream", wantBody: "data: {\"a\":1}\n\ndata: {\"a\":2}\n\n"},
		{name: "An event of lines", headers: http.Header{"Content-Type": {"text/event-stream"}}, body: slices.Values([][]byte{[]byte("a\r\nb\rc\nd")}), wantContentType: "text/event-stream", wantBody: "data: a\ndata: b\ndata: c\ndata: d\n\n"},
		{name: "One value is one event", headers: http.Header{"Content-Type": {"text/event-stream"}}, body: map[string]int{"a": 1}, wantContentType: "text/event-stream", wantBody: "data: {\"a\":1}\n\n"},
		{name: "No events", headers: http.Header{"Content-Type": {"text/event-stream"}}, body: iter.Seq[int](nil), wantContentType: "text/event-stream"},
		{name: "Lines", headers: http.Header{"Content-Type": {"application/x-ndjson"}}, body: slices.Values([]any{1, "two"}), wantContentType: "application/x-ndjson", wantBody: "1\ntwo\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			require.NoError(t, Writer{}.Write(w, http.StatusCreated, tc.headers, tc.body))

			assert.Equal(t, http.StatusCreated, w.Code)
			assert.Equal(t, tc.wantContentType, w.Header().Get("Content-Type"))
			assert.Equal(t, tc.wantBody, w.Body.String())
		})
	}
}

func TestWriterMarshal(t *testing.T) {
	t.Parallel()

	upper := func(v any) ([]byte, error) {
		data, err := json.Marshal(v)
		return bytes.ToUpper(data), err
	}
	tests := []struct {
		name     string
		headers  http.Header
		body     any
		wantBody string
	}{
		{name: "A JSON body", body: map[string]string{"a": "b"}, wantBody: `{"A":"B"}`},
		{name: "Frames", headers: http.Header{"Content-Type": {"application/x-ndjson"}}, body: slices.Values([]map[string]string{{"a": "b"}}), wantBody: "{\"A\":\"B\"}\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			require.NoError(t, Writer{Marshal: upper}.Write(w, http.StatusOK, tc.headers, tc.body))

			assert.Equal(t, tc.wantBody, w.Body.String())
		})
	}
}

func TestWriteErrors(t *testing.T) {
	t.Parallel()

	xml := http.Header{"Content-Type": {"application/xml"}}
	require.Error(t, Writer{}.Write(httptest.NewRecorder(), 200, nil, func() {}))
	require.ErrorIs(t, Writer{}.Write(httptest.NewRecorder(), 200, nil, runtime.NewFileReader(iotest.ErrReader(io.ErrUnexpectedEOF), "", "", -1)), ErrResponseCut)
	require.Error(t, writeFile(httptest.NewRecorder(), 200, runtime.NewFileFromMultipart(&multipart.FileHeader{Filename: "gone"})))
	require.ErrorIs(t, Writer{}.Write(httptest.NewRecorder(), 200, xml, struct{}{}), runtime.ErrContentType)
	require.ErrorIs(t, Writer{}.Write(httptest.NewRecorder(), 200, http.Header{"Content-Type": {"application/x-www-form-urlencoded"}}, 1), runtime.ErrBodyValue)
	require.ErrorIs(t, Writer{}.Write(httptest.NewRecorder(), 200, http.Header{"Content-Type": {"multipart/form-data"}}, 1), runtime.ErrBodyValue)
	require.ErrorIs(t, Writer{}.Write(httptest.NewRecorder(), 200, http.Header{"Content-Type": {"text/plain"}}, make(chan int)), runtime.ErrParamValue)
	require.ErrorIs(t, Writer{}.Write(httptest.NewRecorder(), 200, http.Header{"Content-Type": {"text/event-stream"}}, slices.Values([]func(){nil})), ErrResponseCut)
	require.ErrorIs(t, Writer{}.Write(httptest.NewRecorder(), 200, http.Header{"Content-Type": {"multipart/form-data"}}, struct {
		File runtime.File `json:"file"`
	}{File: runtime.NewFileReader(iotest.ErrReader(io.ErrUnexpectedEOF), "a", "", -1)}), ErrResponseCut)
}

func TestWriteMultipartResponse(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	require.NoError(t, Writer{}.Write(w, http.StatusOK, http.Header{"Content-Type": {"multipart/form-data"}}, struct {
		Title string `json:"title"`
	}{Title: "Cat"}))

	mediaType, params, err := mime.ParseMediaType(w.Header().Get("Content-Type"))
	require.NoError(t, err)
	assert.Equal(t, "multipart/form-data", mediaType)
	form, err := multipart.NewReader(w.Body, params["boundary"]).ReadForm(1 << 10)
	require.NoError(t, err)
	assert.Equal(t, []string{"Cat"}, form.Value["title"])
}

func TestWriteCut(t *testing.T) {
	t.Parallel()

	events := http.Header{"Content-Type": {"text/event-stream"}}
	tests := []struct {
		name    string
		w       *brokenWriter
		headers http.Header
		body    any
	}{
		{name: "Bytes", w: &brokenWriter{isWriteBroken: true}, body: []byte("x")},
		{name: "The first flush", w: &brokenWriter{flushFails: 1}, headers: events, body: slices.Values([]int{1})},
		{name: "A frame", w: &brokenWriter{isWriteBroken: true}, headers: events, body: slices.Values([]int{1})},
		{name: "A flush after a frame", w: &brokenWriter{flushFails: 2}, headers: events, body: slices.Values([]int{1})},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tc.w.ResponseRecorder = httptest.NewRecorder()
			require.ErrorIs(t, Writer{}.Write(tc.w, http.StatusOK, tc.headers, tc.body), ErrResponseCut)
		})
	}
}

func TestWriteFlushesEachFrame(t *testing.T) {
	t.Parallel()

	w := &brokenWriter{ResponseRecorder: httptest.NewRecorder()}
	require.NoError(t, Writer{}.Write(w, http.StatusOK, http.Header{"Content-Type": {"application/jsonl"}}, slices.Values([]int{0, 1})))

	assert.Equal(t, 3, w.flushes)
	assert.Equal(t, "0\n1\n", w.Body.String())
}
