// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

type tagKey struct{}

func TestChainServe(t *testing.T) {
	t.Parallel()

	var built int
	var seen []string
	tag := func(name string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			built++
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = append(seen, name+" "+runtime.OperationID(r.Context()))
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), tagKey{}, name)))
			})
		}
	}
	chain := NewChain(tag("outer"), tag("inner"))
	serve := func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, "serve "+runtime.OperationID(r.Context())+" "+r.Context().Value(tagKey{}).(string))
		w.WriteHeader(http.StatusAccepted)
	}

	for _, id := range []string{"ListPets", "GetPet"} {
		rec := httptest.NewRecorder()
		chain.Serve(rec, httptest.NewRequest(http.MethodGet, "/", nil), id, serve)
		assert.Equal(t, http.StatusAccepted, rec.Code)
	}

	assert.Equal(t, 2, built, "each middleware is built once")
	assert.Equal(t, []string{
		"outer ListPets", "inner ListPets", "serve ListPets inner",
		"outer GetPet", "inner GetPet", "serve GetPet inner",
	}, seen)
}

func TestChainServeWithoutMiddleware(t *testing.T) {
	t.Parallel()

	var got string
	rec := httptest.NewRecorder()

	NewChain().Serve(rec, httptest.NewRequest(http.MethodGet, "/", nil), "ListPets", func(_ http.ResponseWriter, r *http.Request) {
		got = runtime.OperationID(r.Context())
	})

	assert.Equal(t, "ListPets", got)
}

func TestChainServeContextLost(t *testing.T) {
	t.Parallel()

	detach := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(context.Background()))
		})
	}
	chain := NewChain(detach)

	assert.PanicsWithValue(t, ErrContextLost, func() {
		chain.Serve(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil), "ListPets", func(http.ResponseWriter, *http.Request) {})
	})
}
