// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package blocks

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouter(t *testing.T) {
	t.Parallel()

	router := NewRouter(NewPets())
	tests := []struct {
		name     string
		method   string
		path     string
		body     string
		want     int
		wantBody string
	}{
		{name: "Route of the config", method: "GET", path: "/health", want: 200, wantBody: "ok"},
		{name: "Stub of an operation", method: "GET", path: "/pets", want: 501},
		{name: "Stub of an operation with a body", method: "POST", path: "/pets", body: `{"id": 1, "name": "Rex"}`, want: 501},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, tc.want, rec.Code, rec.Body.String())
			assert.Equal(t, tc.wantBody, strings.TrimSpace(rec.Body.String()))
		})
	}
}
