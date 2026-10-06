// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package petstore

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newClient(t *testing.T) *PetClient {
	t.Helper()

	srv := httptest.NewServer(NewRouter(&Service{}))
	t.Cleanup(srv.Close)
	c, err := NewPetClient(srv.URL)
	require.NoError(t, err)
	return c
}

func TestRoundTrip(t *testing.T) {
	t.Parallel()

	c := newClient(t)
	ctx := t.Context()

	created, err := c.CreatePet(ctx, &CreatePetRequestOptions{Body: &NewPet{Name: "Rex"}})
	require.NoError(t, err)
	assert.Equal(t, &Pet{ID: 1, Name: "Rex"}, created)

	pet, err := c.GetPet(ctx, &GetPetRequestOptions{PathParams: &GetPetPathParams{ID: 1}})
	require.NoError(t, err)
	assert.Equal(t, &Pet{ID: 1, Name: "Rex"}, pet)

	_, err = c.GetPet(ctx, &GetPetRequestOptions{PathParams: &GetPetPathParams{ID: 2}})
	var notFound *Error
	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, &Error{Code: http.StatusNotFound, Message: "no such pet"}, notFound)

	missing, err := c.GetPetWithResponse(ctx, &GetPetRequestOptions{PathParams: &GetPetPathParams{ID: 2}})
	require.NoError(t, err)
	assert.Equal(t, http.StatusNotFound, missing.StatusCode())
	assert.Equal(t, &Error{Code: http.StatusNotFound, Message: "no such pet"}, missing.JSON404)
}

func TestToolNames(t *testing.T) {
	t.Parallel()

	server := mcp.NewServer(&mcp.Implementation{Name: "petstore", Version: "1.0.0"}, nil)
	NewMCPTools(newClient(t)).Register(server)

	ctx := t.Context()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Wait() })

	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })

	res, err := session.ListTools(ctx, nil)

	require.NoError(t, err)
	names := make([]string, 0, len(res.Tools))
	for _, tool := range res.Tools {
		names = append(names, tool.Name)
	}
	assert.ElementsMatch(t, []string{"list_pets", "create_pet", "get_pet", "delete_pet"}, names)
}
