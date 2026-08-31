// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package runtime

import (
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
		{name: "Text behind a pointer", body: Ptr("hi"), wantContentType: "application/octet-stream", wantBody: "hi"},
		{name: "Bytes", headers: http.Header{"Content-Type": {"text/plain"}}, body: []byte("hi"), wantContentType: "text/plain", wantBody: "hi"},
		{name: "Bytes behind a pointer", body: Ptr([]byte("hi")), wantContentType: "application/octet-stream", wantBody: "hi"},
		{name: "File", body: NewFile([]byte("data"), "a.bin", "image/png"), wantContentType: "image/png", wantBody: "data"},
		{name: "File pointer", body: Ptr(NewFileReader(strings.NewReader("data"), "a", "", -1)), wantContentType: "", wantBody: "data"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			require.NoError(t, Write(w, http.StatusCreated, tc.headers, tc.body))

			assert.Equal(t, http.StatusCreated, w.Code)
			assert.Equal(t, tc.wantContentType, w.Header().Get("Content-Type"))
			assert.Equal(t, tc.wantBody, w.Body.String())
		})
	}
}

func TestWriteErrors(t *testing.T) {
	t.Parallel()

	require.Error(t, Write(httptest.NewRecorder(), 200, nil, func() {}))
	require.Error(t, Write(httptest.NewRecorder(), 200, nil, NewFileReader(errReader{}, "", "", -1)))
	require.Error(t, WriteFile(httptest.NewRecorder(), 200, NewFileFromMultipart(&multipart.FileHeader{Filename: "gone"})))
}
