// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package defaults

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// service answers with what it was asked.
type service struct{}

func (service) Search(_ context.Context, opts *SearchServiceRequestOptions) (*SearchResponseData, error) {
	asked := &Asked{Q: opts.Query.Q, Limit: opts.Query.Limit}
	if opts.Query.Sort != nil {
		asked.Sort = new(string(*opts.Query.Sort))
	}
	return NewSearchResponseData(asked), nil
}

// newSession serves the service over HTTP, registers the tools on an MCP server that calls it
// through the client, and connects an MCP client in process. Registering panics on a default that
// does not fit its schema.
func newSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	server := mcp.NewServer(&mcp.Implementation{Name: "search", Version: "1"}, nil)
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

// TestListTools checks that the default of sort stays and the string default of the integer
// limit is left out.
func TestListTools(t *testing.T) {
	t.Parallel()

	session := newSession(t)

	res, err := session.ListTools(t.Context(), nil)

	require.NoError(t, err)
	require.Len(t, res.Tools, 1)
	assert.Equal(t, map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"q"},
		"properties": map[string]any{
			"q":     map[string]any{"type": "string", "description": "What to look for."},
			"sort":  map[string]any{"type": "string", "enum": []any{"name", "date"}, "default": "name", "description": "The order of the results."},
			"limit": map[string]any{"type": "integer", "description": "How many results to return at most."},
		},
	}, res.Tools[0].InputSchema)
}

// TestCallTool checks that the SDK fills in the default the input schema keeps.
func TestCallTool(t *testing.T) {
	t.Parallel()

	session := newSession(t)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "search", Arguments: map[string]any{"q": "lamp"}})

	require.NoError(t, err)
	assert.False(t, res.IsError)
	assert.Equal(t, map[string]any{"q": "lamp", "sort": "name"}, res.StructuredContent)
}
