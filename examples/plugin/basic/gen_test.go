// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package basic

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoutesAndBodies(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []Route{
		{ID: "ListPets", Method: "GET", Path: "/pets", Status: 200},
		{ID: "CreatePet", Method: "POST", Path: "/pets", Status: 201},
		{ID: "DeletePet", Method: "DELETE", Path: "/pets/{id}", Status: 204},
		{ID: "Ping", Method: "GET", Path: "/ping", Status: 200},
	}, Routes)

	assert.IsType(t, (*ListPetsResponse200)(nil), Bodies["ListPets"]())
	assert.IsType(t, (**Pet)(nil), Bodies["CreatePet"]())
	assert.IsType(t, (**PingResponse200)(nil), Bodies["Ping"]())
	assert.NotContains(t, Bodies, "DeletePet")
}

func TestRegister(t *testing.T) {
	t.Parallel()

	router := chi.NewRouter()
	Register(router, "/_routes")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, httptest.NewRequest("GET", "/_routes", nil))

	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, `[
		{"ID": "ListPets", "Method": "GET", "Path": "/pets", "Status": 200},
		{"ID": "CreatePet", "Method": "POST", "Path": "/pets", "Status": 201},
		{"ID": "DeletePet", "Method": "DELETE", "Path": "/pets/{id}", "Status": 204},
		{"ID": "Ping", "Method": "GET", "Path": "/ping", "Status": 200}
	]`, rec.Body.String())
}

func TestServiceScaffold(t *testing.T) {
	t.Parallel()

	svc := NewPets()
	pets := ListPetsResponse200{{ID: 1, Name: "Rex"}}

	res, err := svc.ListPets(context.Background(), &ListPetsServiceRequestOptions{GenerateResponse: func() any { return pets }})
	require.NoError(t, err)
	assert.Equal(t, &ListPetsResponseData{Status: 200, Body: pets}, res)

	created, err := svc.CreatePet(context.Background(), &CreatePetServiceRequestOptions{GenerateResponse: func() any { return &pets[0] }})
	require.NoError(t, err)
	assert.Equal(t, &CreatePetResponseData{Status: 201, Body: &pets[0]}, created)

	_, err = svc.ListPets(context.Background(), &ListPetsServiceRequestOptions{})
	require.ErrorIs(t, err, ErrNotImplemented)

	rec := httptest.NewRecorder()
	NewRouter(svc).ServeHTTP(rec, httptest.NewRequest("GET", "/pets", nil))
	assert.Equal(t, 500, rec.Code, "the handlers set no GenerateResponse")
}

func TestWithBodies(t *testing.T) {
	t.Parallel()

	router := NewRouter(WithBodies(NewPets()))
	tests := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{name: "Operation with a JSON body", method: "GET", path: "/pets", want: 200},
		{name: "Operation that takes a body", method: "POST", path: "/pets", body: `{"id": 1, "name": "Rex"}`, want: 201},
		{name: "Operation without a body", method: "DELETE", path: "/pets/1", want: 204},
		{name: "Operation with a text body", method: "GET", path: "/ping", want: 200},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, tc.want, rec.Code, rec.Body.String())
		})
	}
}
