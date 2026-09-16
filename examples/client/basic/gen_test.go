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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

var errBoom = errors.New("boom")

// service keeps pets in memory.
type service struct {
	pets map[int]Pet
}

func (s *service) ListPets(_ context.Context, opts *ListPetsServiceRequestOptions) (*ListPetsResponseData, error) {
	pets := ListPetsResponse200{}
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

// newClient serves the service and returns a client of it.
func newClient(t *testing.T, opts ...PetClientOption) *PetClient {
	t.Helper()

	srv := httptest.NewServer(NewRouter(&service{pets: map[int]Pet{}}))
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

	var seen []string
	record := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			seen = append(seen, r.Header.Get("X-Trace")+" "+r.UserAgent())
			next.ServeHTTP(w, r)
		})
	}
	srv := httptest.NewServer(NewRouter(&service{pets: map[int]Pet{}}, WithMiddleware(record)))
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
	assert.Equal(t, []string{"abc pets/1"}, seen, "the editors run in order")
}

func TestRequestEditorError(t *testing.T) {
	t.Parallel()

	failing := errors.New("no token")
	c := newClient(t, WithRequestEditor(func(context.Context, *http.Request) error { return failing }))

	_, err := c.Ping(context.Background(), nil)

	require.ErrorIs(t, err, failing)
}

func TestRequestAlone(t *testing.T) {
	t.Parallel()

	c, err := NewPetClient("https://api.example.test/v1")
	require.NoError(t, err)

	req, err := c.GetPetRequest(context.Background(), &GetPetRequestOptions{PathParams: &GetPetPathParams{ID: 7}})

	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, req.Method)
	assert.Equal(t, "https://api.example.test/v1/pets/7", req.URL.String())
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

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}
