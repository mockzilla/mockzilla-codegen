// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package basic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

// petShop keeps pets in memory and fails to create a pet named boom.
type petShop struct {
	mu   sync.Mutex
	pets map[int]Pet
}

func (s *petShop) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /pets", s.list)
	mux.HandleFunc("POST /pets", s.create)
	mux.HandleFunc("GET /pets/{id}", s.get)
	mux.HandleFunc("DELETE /pets/{id}", s.remove)
	mux.HandleFunc("GET /ping", ping)
	return mux
}

func (s *petShop) list(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	found := make([]Pet, 0, len(s.pets))
	for _, id := range slices.Sorted(maps.Keys(s.pets)) {
		found = append(found, s.pets[id])
	}
	if limit, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && limit >= 0 && limit < len(found) {
		found = found[:limit]
	}
	writeJSON(w, http.StatusOK, found)
}

func (s *petShop) create(w http.ResponseWriter, r *http.Request) {
	var pet Pet
	if err := json.NewDecoder(r.Body).Decode(&pet); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if pet.Name == "boom" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":"internal server error"}`)
		return
	}

	s.mu.Lock()
	s.pets[pet.ID] = pet
	s.mu.Unlock()
	w.Header().Set("Location", "/pets/1")
	writeJSON(w, http.StatusCreated, pet)
}

func (s *petShop) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	pet, ok := s.pets[id]
	s.mu.Unlock()
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, pet)
}

func (s *petShop) remove(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	delete(s.pets, id)
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func ping(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	_, _ = io.WriteString(w, "pong")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func orderEditor(name string) RequestEditor {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Add("X-Order", name)
		return nil
	}
}

func newClient(t *testing.T, opts ...PetClientOption) *PetClient {
	t.Helper()

	srv := httptest.NewServer((&petShop{pets: map[int]Pet{}}).handler())
	t.Cleanup(srv.Close)
	c, err := NewPetClient(srv.URL, opts...)
	require.NoError(t, err)
	return c
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClient(t)

	created, err := c.CreatePet(ctx, &CreatePetRequestOptions{Body: &Pet{ID: 1, Name: "Rex"}})
	require.NoError(t, err)
	assert.Equal(t, &Pet{ID: 1, Name: "Rex"}, created)

	pet, err := c.GetPet(ctx, &GetPetRequestOptions{PathParams: &GetPetPathParams{ID: 1}})
	require.NoError(t, err)
	assert.Equal(t, &Pet{ID: 1, Name: "Rex"}, pet)

	pets, err := c.ListPets(ctx, &ListPetsRequestOptions{Query: &ListPetsQuery{Limit: new(0)}})
	require.NoError(t, err)
	assert.Equal(t, ListPetsResponse200{}, pets)

	pets, err = c.ListPets(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, ListPetsResponse200{{ID: 1, Name: "Rex"}}, pets)

	require.NoError(t, c.DeletePet(ctx, &DeletePetRequestOptions{PathParams: &DeletePetPathParams{ID: 1}}))

	pong, err := c.Ping(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, "pong", *pong)
}

func TestErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClient(t)
	tests := []struct {
		name       string
		call       func() error
		wantStatus int
		wantBody   string
		wantErr    error
	}{
		{
			name: "A status the spec documents without a body",
			call: func() error {
				_, err := c.GetPet(ctx, &GetPetRequestOptions{PathParams: &GetPetPathParams{ID: 2}})
				return err
			},
			wantStatus: http.StatusNotFound,
		},
		{
			name: "A status the spec does not document",
			call: func() error {
				_, err := c.CreatePet(ctx, &CreatePetRequestOptions{Body: &Pet{ID: 2, Name: "boom"}})
				return err
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   `{"error":"internal server error"}`,
		},
		{
			name:    "A required body left out never reaches the server",
			call:    func() error { _, err := c.CreatePet(ctx, nil); return err },
			wantErr: runtime.ErrBodyEmpty,
		},
		{
			name:    "A path parameter group left out",
			call:    func() error { _, err := c.GetPet(ctx, nil); return err },
			wantErr: runtime.ErrParamMissing,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.call()

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			var apiErr *runtime.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, tc.wantStatus, apiErr.Status)
			assert.Equal(t, tc.wantBody, string(apiErr.Body))
			assert.NoError(t, apiErr.Err, "no error type is documented")
		})
	}
}

func TestOptions(t *testing.T) {
	t.Parallel()

	seen := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Get("X-Trace") + " " + r.UserAgent()
		ping(w, r)
	}))
	t.Cleanup(srv.Close)
	var sent int
	doer := doerFunc(func(req *http.Request) (*http.Response, error) {
		sent++
		return http.DefaultClient.Do(req)
	})
	trace := func(_ context.Context, req *http.Request) error {
		req.Header.Set("X-Trace", "abc")
		return nil
	}
	agent := func(_ context.Context, req *http.Request) error {
		req.Header.Set("User-Agent", "pets/1")
		return nil
	}
	c, err := NewPetClient(srv.URL, WithHTTPClient(doer), WithRequestEditor(trace), WithRequestEditor(agent))
	require.NoError(t, err)

	_, err = c.Ping(context.Background(), nil)

	require.NoError(t, err)
	assert.Equal(t, 1, sent, "the doer sends")
	assert.Equal(t, "abc pets/1", <-seen, "the editors run in order")
}

func TestCallEditors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	seen := make(chan []string, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Values("X-Order")
		ping(w, r)
	}))
	t.Cleanup(srv.Close)
	c, err := NewPetClient(srv.URL, WithRequestEditor(orderEditor("client 1")), WithRequestEditor(orderEditor("client 2")))
	require.NoError(t, err)

	_, err = c.Ping(ctx, nil, orderEditor("call 1"), orderEditor("call 2"))
	require.NoError(t, err)
	assert.Equal(t, []string{"client 1", "client 2", "call 1", "call 2"}, <-seen, "the editors of the call run after those of the client")

	_, err = c.Ping(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"client 1", "client 2"}, <-seen, "the editors of a call stay with that call")
}

func TestRequestEditorError(t *testing.T) {
	t.Parallel()

	failing := errors.New("no token")
	fail := func(context.Context, *http.Request) error { return failing }
	tests := []struct {
		name    string
		options []PetClientOption
		editors []RequestEditor
	}{
		{name: "An editor of the client fails", options: []PetClientOption{WithRequestEditor(fail)}},
		{name: "An editor of the call fails", editors: []RequestEditor{fail}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			c := newClient(t, tc.options...)

			_, err := c.Ping(context.Background(), nil, tc.editors...)

			require.ErrorIs(t, err, failing)
		})
	}
}

func TestRequestAlone(t *testing.T) {
	t.Parallel()

	c, err := NewPetClient("https://api.example.test/v1", WithRequestEditor(orderEditor("client")))
	require.NoError(t, err)

	req, err := c.GetPetRequest(context.Background(), &GetPetRequestOptions{PathParams: &GetPetPathParams{ID: 7}}, orderEditor("call"))

	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, req.Method)
	assert.Equal(t, "https://api.example.test/v1/pets/7", req.URL.String())
	assert.Equal(t, []string{"client", "call"}, req.Header.Values("X-Order"), "the request carries the edits of the client, then the call")
}

func TestNewPetClientRejectsABareHost(t *testing.T) {
	t.Parallel()

	_, err := NewPetClient("api.example.test")

	require.ErrorIs(t, err, runtime.ErrBaseURL)
}

func TestInterface(t *testing.T) {
	t.Parallel()

	var c PetClientInterface = newClient(t)

	pong, err := c.Ping(context.Background(), nil)

	require.NoError(t, err)
	assert.Equal(t, "pong", *pong)
}
