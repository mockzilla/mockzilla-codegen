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
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/examples/server/internal/servertest"
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

	servertest.Run(t, NewRouter(&service{pets: map[int]Pet{}}), servertest.Basic("404 page not found\n"))
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

func TestAdapterAlone(t *testing.T) {
	t.Parallel()

	adapter := NewHTTPAdapter(&service{pets: map[int]Pet{}})
	rec := httptest.NewRecorder()

	adapter.Ping(rec, httptest.NewRequest("GET", "/ping", nil))

	assert.Equal(t, 200, rec.Code)
	assert.Equal(t, "pong", rec.Body.String())
}
