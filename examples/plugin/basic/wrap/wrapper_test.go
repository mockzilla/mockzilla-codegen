// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package wrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/examples/plugin/basic"
)

func TestNewRouterWithBodies(t *testing.T) {
	t.Parallel()

	router := NewRouterWithBodies(basic.NewPets())
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

func TestNewRouterWithBodiesTakesOptions(t *testing.T) {
	t.Parallel()

	tagged := basic.WithMiddleware(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Wrapped", "yes")
			next.ServeHTTP(w, r)
		})
	})
	rec := httptest.NewRecorder()

	NewRouterWithBodies(basic.NewPets(), tagged).ServeHTTP(rec, httptest.NewRequest("GET", "/ping", nil))

	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, "yes", rec.Header().Get("X-Wrapped"))
}

func TestWithBodiesMakesTheResponseOfEachOperation(t *testing.T) {
	t.Parallel()

	svc := WithBodies(basic.NewPets())

	listed, err := svc.ListPets(context.Background(), &basic.ListPetsServiceRequestOptions{})
	require.NoError(t, err)
	assert.Equal(t, basic.NewListPetsResponseData(nil), listed)

	made, err := svc.CreatePet(context.Background(), &basic.CreatePetServiceRequestOptions{})
	require.NoError(t, err)
	assert.Equal(t, basic.NewCreatePetResponseData(&basic.Pet{}), made)

	gone, err := svc.DeletePet(context.Background(), &basic.DeletePetServiceRequestOptions{})
	require.NoError(t, err)
	assert.Equal(t, basic.NewDeletePetResponseData(), gone)

	pong, err := svc.Ping(context.Background(), &basic.PingServiceRequestOptions{})
	require.NoError(t, err)
	assert.Equal(t, basic.NewPingResponseData(new(basic.PingResponse200)), pong)
}
