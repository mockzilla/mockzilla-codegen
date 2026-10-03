// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package wrapper

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWithBodies(t *testing.T) {
	t.Parallel()

	router := NewRouter(WithBodies(NewPets()))
	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		want     int
		wantType string
		wantBody string
	}{
		{name: "Operation with a JSON list", method: "GET", path: "/pets", want: 200, wantType: "application/json", wantBody: "null"},
		{
			name: "Operation with a JSON object", method: "POST", path: "/pets", body: `{"id": 1, "name": "Rex"}`,
			want: 201, wantType: "application/json", wantBody: `{"id":0,"name":""}`,
		},
		{name: "Operation without a body", method: "DELETE", path: "/pets/1", want: 204},
		{name: "Operation with a text body", method: "GET", path: "/ping", want: 200, wantType: "text/plain"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, tc.want, rec.Code, rec.Body.String())
			assert.Equal(t, tc.wantType, rec.Header().Get("Content-Type"))
			assert.Equal(t, tc.wantBody, strings.TrimSpace(rec.Body.String()))
		})
	}
}
