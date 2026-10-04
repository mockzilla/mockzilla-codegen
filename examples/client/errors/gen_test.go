// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package errors

import (
	"context"
	stderrors "errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// service answers by id: 1 is a pet, 2 a pet the spec rejects, 3 is locked, anything else a
// Problem.
type service struct{}

func (service) AddPet(_ context.Context, opts *AddPetServiceRequestOptions) (*AddPetResponseData, error) {
	return NewAddPetResponseData201(opts.Body), nil
}

func (service) GetPet(_ context.Context, opts *GetPetServiceRequestOptions) (*GetPetResponseData, error) {
	switch id := opts.PathParams.ID; id {
	case 1:
		return NewGetPetResponseData200(&Pet{Name: "Rex", Age: new(3)}), nil
	case 2:
		return NewGetPetResponseData200(&Pet{Name: ""}), nil
	case 3:
		return NewGetPetResponseData409(&Locked{Detail: new("locked"), Until: new("later")}), nil
	default:
		return nil, NewProblem(fmt.Sprintf("no pet %d", id))
	}
}

func (service) PutPet(_ context.Context, opts *PutPetServiceRequestOptions) (*PutPetResponseData, error) {
	return NewPutPetResponseData(opts.Body), nil
}

func newClient(t *testing.T) *Client {
	t.Helper()

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)
	return c
}

func TestErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	c := newClient(t)
	tests := []struct {
		name        string
		id          int
		wantStatus  int
		wantProblem *Problem
		wantBody    string
		wantMessage string
	}{
		{name: "A typed error comes back as itself", id: 9, wantStatus: http.StatusNotFound, wantProblem: &Problem{Detail: "no pet 9"}, wantMessage: "unexpected status 404 Not Found: no pet 9"},
		{name: "A response type that is no error keeps its raw body", id: 3, wantStatus: http.StatusConflict, wantBody: `{"detail":"locked","until":"later"}`, wantMessage: "unexpected status 409 Conflict"},
		{name: "A server failure the spec does not document", id: 2, wantStatus: http.StatusInternalServerError, wantBody: `{"error":"invalid response: name: must be at least 1 characters long"}`, wantMessage: "unexpected status 500 Internal Server Error"},
		{name: "A request the server rejects", id: 0, wantStatus: http.StatusBadRequest, wantBody: `{"error":"invalid request: path.id: must be at least 1"}`, wantMessage: "unexpected status 400 Bad Request"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pet, err := c.GetPet(ctx, &GetPetRequestOptions{PathParams: &GetPetPathParams{ID: tc.id}})

			assert.Nil(t, pet)
			var apiErr *runtime.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, tc.wantStatus, apiErr.Status)
			assert.EqualError(t, err, tc.wantMessage)
			var problem *Problem
			if tc.wantProblem != nil {
				require.ErrorAs(t, err, &problem)
				assert.Equal(t, tc.wantProblem, problem)
				return
			}
			assert.False(t, stderrors.As(err, &problem))
			assert.Equal(t, tc.wantBody, string(apiErr.Body))
		})
	}
}

func TestSuccessStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		status      int
		body        string
		want        *Pet
		wantMessage string
	}{
		{name: "The 2xx whose body the method returns", status: http.StatusCreated, body: `{"name":"Rex"}`, want: &Pet{Name: "Rex"}},
		{name: "Another listed 2xx", status: http.StatusNoContent},
		{name: "A 2xx the spec does not list", status: http.StatusAccepted, body: `{"detail":"queued"}`, wantMessage: "unexpected status 202 Accepted"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)
			c, err := NewClient(srv.URL)
			require.NoError(t, err)

			pet, err := c.AddPet(context.Background(), &AddPetRequestOptions{Body: &Pet{Name: "Rex"}})

			assert.Equal(t, tc.want, pet)
			if tc.wantMessage == "" {
				require.NoError(t, err)
				return
			}
			var apiErr *runtime.APIError
			require.ErrorAs(t, err, &apiErr)
			assert.EqualError(t, err, tc.wantMessage)
			assert.Equal(t, tc.body, string(apiErr.Body))
			var problem *Problem
			assert.False(t, stderrors.As(err, &problem))
		})
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	opts := &PutPetRequestOptions{PathParams: &PutPetPathParams{ID: 0}, Body: &Pet{Name: ""}}

	err := opts.Validate()

	require.EqualError(t, err, "path.id: must be at least 1; body.name: must be at least 1 characters long")
	require.NoError(t, (&PutPetRequestOptions{PathParams: &PutPetPathParams{ID: 1}, Body: &Pet{Name: "Rex"}}).Validate())
}

func TestValidBody(t *testing.T) {
	t.Parallel()

	c := newClient(t)

	pet, err := c.PutPet(context.Background(), &PutPetRequestOptions{PathParams: &PutPetPathParams{ID: 1}, Body: &Pet{Name: "Rex"}})

	require.NoError(t, err)
	assert.Equal(t, &Pet{Name: "Rex"}, pet)
}
