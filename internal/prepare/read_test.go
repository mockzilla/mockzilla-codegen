// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package prepare

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRead(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/spec.yaml":
			_, _ = w.Write([]byte("openapi: 3.1.0\n"))
		case "/short.yaml":
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("openapi"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	closed := httptest.NewServer(http.NotFoundHandler())
	closed.Close()

	tests := []struct {
		name    string
		loc     string
		want    string
		wantErr string
	}{
		{name: "File", loc: filepath.Join("testdata", "split", "pet.yaml"), want: "type: object\nproperties:\n  name: {type: string}\n"},
		{name: "Missing file", loc: filepath.Join("testdata", "missing.yaml"), wantErr: "no such file"},
		{name: "URL", loc: srv.URL + "/spec.yaml", want: "openapi: 3.1.0\n"},
		{name: "Not found", loc: srv.URL + "/missing.yaml", wantErr: "404 Not Found"},
		{name: "Body cut short", loc: srv.URL + "/short.yaml", wantErr: "unexpected EOF"},
		{name: "Server down", loc: closed.URL + "/spec.yaml", wantErr: "connection refused"},
		{name: "Bad URL", loc: "http://example.com/%zz", wantErr: "invalid URL escape"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := read(context.Background(), tt.loc)
			if tt.wantErr != "" {
				require.ErrorIs(t, err, ErrRead)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, string(got))
		})
	}
}
