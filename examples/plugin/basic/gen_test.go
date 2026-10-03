// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package basic

import (
	"context"
	"errors"
	"net/http/httptest"
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
