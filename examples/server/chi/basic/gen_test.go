// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package basic

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errBoom = errors.New("boom")

// service keeps pets in memory.
type service struct {
	pets map[int]Pet
}

func (s *service) ListPets(_ context.Context, opts *ListPetsServiceRequestOptions) (*ListPetsResponseData, error) {
	var pets ListPetsResponse200
	for _, p := range s.pets {
		pets = append(pets, p)
	}
	if opts.Query.Limit != nil && *opts.Query.Limit < len(pets) {
		pets = pets[:*opts.Query.Limit]
	}
	return NewListPetsResponseData(pets), nil
}

func (s *service) CreatePet(_ context.Context, opts *CreatePetServiceRequestOptions) (*CreatePetResponseData, error) {
	if opts.Body.Name == "boom" {
		return nil, errBoom
	}
	s.pets[opts.Body.ID] = *opts.Body
	return NewCreatePetResponseData(opts.Body).WithTypedHeaders(CreatePetResponse201Headers{Location: new("/pets/1")}), nil
}

func (s *service) GetPet(_ context.Context, opts *GetPetServiceRequestOptions) (*GetPetResponseData, error) {
	p, ok := s.pets[opts.PathParams.ID]
	if !ok {
		return NewGetPetResponseData404(), nil
	}
	return NewGetPetResponseData200(&p), nil
}

func (s *service) DeletePet(_ context.Context, opts *DeletePetServiceRequestOptions) (*DeletePetResponseData, error) {
	delete(s.pets, opts.PathParams.ID)
	return NewDeletePetResponseData(), nil
}

func (*service) Ping(context.Context, *PingServiceRequestOptions) (*PingResponseData, error) {
	return NewPingResponseData(new("pong")), nil
}

func TestRouter(t *testing.T) {
	t.Parallel()

	router := NewRouter(&service{pets: map[int]Pet{}})
	tests := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
		wantStatus  int
		wantBody    string
	}{
		{name: "Create a pet", method: "POST", path: "/pets", body: `{"id":1,"name":"Rex"}`, contentType: "application/json", wantStatus: 201, wantBody: `{"id":1,"name":"Rex"}`},
		{name: "Get the pet", method: "GET", path: "/pets/1", wantStatus: 200, wantBody: `{"id":1,"name":"Rex"}`},
		{name: "List with a limit", method: "GET", path: "/pets?limit=0", wantStatus: 200, wantBody: `[]`},
		{name: "No such pet", method: "GET", path: "/pets/2", wantStatus: 404, wantBody: ``},
		{name: "A path parameter that is not a number", method: "GET", path: "/pets/x", wantStatus: 400, wantBody: `{"error":"invalid path parameter \"id\": invalid parameter value: \"x\" is no int"}`},
		{name: "A body in another media type", method: "POST", path: "/pets", body: `<pet/>`, contentType: "application/xml", wantStatus: 415, wantBody: `{"error":"invalid request body: unsupported content type: application/xml"}`},
		{name: "A required body left out", method: "POST", path: "/pets", wantStatus: 400, wantBody: `{"error":"invalid request body: request body is required"}`},
		{name: "A body that is not JSON", method: "POST", path: "/pets", body: `{`, contentType: "application/json", wantStatus: 400, wantBody: `{"error":"invalid request body: unexpected end of JSON input"}`},
		{name: "The service fails", method: "POST", path: "/pets", body: `{"id":2,"name":"boom"}`, contentType: "application/json", wantStatus: 500, wantBody: `{"error":"internal server error"}`},
		{name: "Delete the pet", method: "DELETE", path: "/pets/1", wantStatus: 204, wantBody: ``},
		{name: "Text response", method: "GET", path: "/ping", wantStatus: 200, wantBody: `pong`},
		{name: "Unknown route", method: "GET", path: "/nope", wantStatus: 404, wantBody: "404 page not found\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantStatus, rec.Code)
			assert.Equal(t, tc.wantBody, rec.Body.String())
		})
	}
}

func TestResponseHeaders(t *testing.T) {
	t.Parallel()

	router := NewRouter(&service{pets: map[int]Pet{}})
	req := httptest.NewRequest("POST", "/pets", strings.NewReader(`{"id":1,"name":"Rex"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, "/pets/1", rec.Header().Get("Location"))
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
}

func TestWithRouterAndMiddleware(t *testing.T) {
	t.Parallel()

	var seen []string
	tag := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = append(seen, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	own := chi.NewRouter()
	own.Get("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	router := NewRouter(&service{pets: map[int]Pet{}}, WithRouter(own), WithMiddleware(tag("outer"), tag("inner")))
	require.Same(t, own, router)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/ping", nil))
	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, []string{"outer", "inner"}, seen)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/health", nil))
	assert.Equal(t, 204, rec.Code)
	assert.Equal(t, []string{"outer", "inner"}, seen, "the middleware wraps the generated routes only")
}

func TestErrorHandler(t *testing.T) {
	t.Parallel()

	handler := ErrorHandlerFunc(func(w http.ResponseWriter, _ *http.Request, status int, err error) {
		var herr *HandlerError
		require.ErrorAs(t, err, &herr)
		w.Header().Set("X-Kind", herr.Kind.String())
		w.Header().Set("X-Operation", herr.OperationID)
		w.WriteHeader(status)
	})
	router := NewRouter(&service{pets: map[int]Pet{}}, WithErrorHandler(handler))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest("GET", "/pets/x", nil))

	assert.Equal(t, 400, rec.Code)
	assert.Equal(t, "parse", rec.Header().Get("X-Kind"))
	assert.Equal(t, "GetPet", rec.Header().Get("X-Operation"))
}

func TestTextErrors(t *testing.T) {
	t.Parallel()

	router := NewRouter(&service{pets: map[int]Pet{}})
	req := httptest.NewRequest("GET", "/pets/x", nil)
	req.Header.Set("Accept", "text/plain")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, 400, rec.Code)
	assert.Equal(t, "text/plain; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, `invalid path parameter "id": invalid parameter value: "x" is no int`, rec.Body.String())
}

func TestAdapterAlone(t *testing.T) {
	t.Parallel()

	adapter := NewHTTPAdapter(&service{pets: map[int]Pet{}})
	rec := httptest.NewRecorder()

	adapter.Ping(rec, httptest.NewRequest("GET", "/ping", nil))

	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, "pong", rec.Body.String())
}
