// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package petstore

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(NewRouter(&Service{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)
	ctx := t.Context()

	created, err := c.CreatePet(ctx, &CreatePetRequestOptions{Body: &NewPet{Name: "Rex", Tag: new("dog")}})
	require.NoError(t, err)
	assert.Equal(t, &Pet{ID: 1, Name: "Rex", Tag: new("dog")}, created)

	pet, err := c.GetPet(ctx, &GetPetRequestOptions{PathParams: &GetPetPathParams{ID: 1}})
	require.NoError(t, err)
	assert.Equal(t, &Pet{ID: 1, Name: "Rex", Tag: new("dog")}, pet)

	_, err = c.GetPet(ctx, &GetPetRequestOptions{PathParams: &GetPetPathParams{ID: 2}})
	var notFound *Error
	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, &Error{Code: http.StatusNotFound, Message: "no such pet"}, notFound)

	_, err = c.CreatePet(ctx, &CreatePetRequestOptions{Body: &NewPet{Name: "Tom"}})
	require.NoError(t, err)
	pets, err := c.ListPets(ctx, &ListPetsRequestOptions{Query: &ListPetsQuery{Limit: new(int32(1))}})
	require.NoError(t, err)
	assert.Equal(t, ListPetsResponse200{{ID: 1, Name: "Rex", Tag: new("dog")}}, pets)
}
