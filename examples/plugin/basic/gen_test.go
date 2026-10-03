// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package basic

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errGenerate = errors.New("nothing to generate")

func TestRoutes(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []Route{
		{ID: "ListPets", Method: "GET", Path: "/pets", Status: 200},
		{ID: "CreatePet", Method: "POST", Path: "/pets", Status: 201},
		{ID: "DeletePet", Method: "DELETE", Path: "/pets/{id}", Status: 204},
		{ID: "Ping", Method: "GET", Path: "/ping", Status: 200},
	}, Routes)
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
	listed := &ListPetsResponseData{Status: 200, Body: ListPetsResponse200{{ID: 1, Name: "Rex"}}}
	created := &CreatePetResponseData{Status: 201, Body: &Pet{ID: 1, Name: "Rex"}}

	res, err := svc.ListPets(context.Background(), &ListPetsServiceRequestOptions{
		GenerateResponse: func() (*ListPetsResponseData, error) { return listed, nil },
	})
	require.NoError(t, err)
	assert.Same(t, listed, res)

	made, err := svc.CreatePet(context.Background(), &CreatePetServiceRequestOptions{
		GenerateResponse: func() (*CreatePetResponseData, error) { return created, nil },
	})
	require.NoError(t, err)
	assert.Same(t, created, made)

	_, err = svc.DeletePet(context.Background(), &DeletePetServiceRequestOptions{
		GenerateResponse: func() (*DeletePetResponseData, error) { return nil, errGenerate },
	})
	require.ErrorIs(t, err, errGenerate)

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

func TestWithBodiesMakesTheResponseOfEachOperation(t *testing.T) {
	t.Parallel()

	svc := WithBodies(NewPets())

	listed, err := svc.ListPets(context.Background(), &ListPetsServiceRequestOptions{})
	require.NoError(t, err)
	assert.Equal(t, NewListPetsResponseData(nil), listed)

	made, err := svc.CreatePet(context.Background(), &CreatePetServiceRequestOptions{})
	require.NoError(t, err)
	assert.Equal(t, NewCreatePetResponseData(&Pet{}), made)

	gone, err := svc.DeletePet(context.Background(), &DeletePetServiceRequestOptions{})
	require.NoError(t, err)
	assert.Equal(t, NewDeletePetResponseData(), gone)

	pong, err := svc.Ping(context.Background(), &PingServiceRequestOptions{})
	require.NoError(t, err)
	assert.Equal(t, NewPingResponseData(new(PingResponse200)), pong)
}
