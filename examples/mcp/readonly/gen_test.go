// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package readonly

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// service stores the pet it was sent under the id 7.
type service struct{}

func (service) AddPet(_ context.Context, opts *AddPetServiceRequestOptions) (*AddPetResponseData, error) {
	return NewAddPetResponseData(&Pet{ID: 7, Name: opts.Body.Name, CreatedAt: new(time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC))}), nil
}

// newSession serves the service over HTTP, registers the tools on an MCP server that calls it
// through the client, and connects an MCP client in process.
func newSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	server := mcp.NewServer(&mcp.Implementation{Name: "pets", Version: "1"}, nil)
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

// TestListTools checks that the readOnly id and createdAt are not in the input, and that the
// member which requires the id of the other member no longer does.
func TestListTools(t *testing.T) {
	t.Parallel()

	session := newSession(t)

	res, err := session.ListTools(t.Context(), nil)

	require.NoError(t, err)
	require.Len(t, res.Tools, 1)
	assert.Equal(t, map[string]any{
		"Pet": map[string]any{"allOf": []any{
			map[string]any{"$ref": "#/$defs/Stored"},
			map[string]any{"type": "object", "properties": map[string]any{"name": map[string]any{"type": "string"}}, "required": []any{"name"}},
		}},
		"Stored": map[string]any{"type": "object"},
	}, res.Tools[0].InputSchema.(map[string]any)["$defs"])
}

// TestCallTool checks that a pet without an id is taken, and the answer holds the id the server
// gave it.
func TestCallTool(t *testing.T) {
	t.Parallel()

	session := newSession(t)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "add_pet", Arguments: map[string]any{"body": map[string]any{"name": "Rex"}}})

	require.NoError(t, err)
	assert.False(t, res.IsError, res.Content)
	assert.Equal(t, map[string]any{"id": float64(7), "name": "Rex", "createdAt": "2026-10-04T00:00:00Z"}, res.StructuredContent)
}
