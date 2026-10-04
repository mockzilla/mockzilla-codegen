// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package discriminator

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// service stores the animal it was sent as it is.
type service struct{}

func (service) AddAnimal(_ context.Context, opts *AddAnimalServiceRequestOptions) (*AddAnimalResponseData, error) {
	return NewAddAnimalResponseData(opts.Body), nil
}

// newSession serves the service over HTTP, registers the tools on an MCP server that calls it
// through the client, and connects an MCP client in process.
func newSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	server := mcp.NewServer(&mcp.Implementation{Name: "animals", Version: "1"}, nil)
	NewMCPTools(c).Register(server)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx := t.Context()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Wait() })

	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// TestListTools checks that each variant requires kind and takes the value the mapping lists for
// it.
func TestListTools(t *testing.T) {
	t.Parallel()

	session := newSession(t)

	res, err := session.ListTools(t.Context(), nil)

	require.NoError(t, err)
	require.Len(t, res.Tools, 1)
	pinned := func(name, value string) map[string]any {
		return map[string]any{
			"$ref":       "#/$defs/" + name,
			"properties": map[string]any{"kind": map[string]any{"const": value}},
			"required":   []any{"kind"},
		}
	}
	assert.Equal(t, map[string]any{"oneOf": []any{pinned("Cat", "cat"), pinned("Dog", "dog")}}, res.Tools[0].InputSchema.(map[string]any)["$defs"].(map[string]any)["Animal"])
}

// TestCallTool checks that a cat and a dog of the same shape are each taken, and that a body the
// API cannot tell apart is an error the assistant sees.
func TestCallTool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    map[string]any
		want    map[string]any
		isError bool
	}{
		{name: "A cat", body: map[string]any{"kind": "cat", "name": "Tom"}, want: map[string]any{"kind": "cat", "name": "Tom"}},
		{name: "A dog", body: map[string]any{"kind": "dog", "name": "Rex", "breed": "collie"}, want: map[string]any{"kind": "dog", "name": "Rex", "breed": "collie"}},
		{name: "A kind the mapping does not list", body: map[string]any{"kind": "cow", "name": "Bess"}, isError: true},
		{name: "No kind", body: map[string]any{"name": "Rex"}, isError: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			session := newSession(t)

			res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "add_animal", Arguments: map[string]any{"body": tc.body}})

			require.NoError(t, err)
			assert.Equal(t, tc.isError, res.IsError, res.Content)
			if !tc.isError {
				assert.Equal(t, tc.want, res.StructuredContent)
			}
		})
	}
}
