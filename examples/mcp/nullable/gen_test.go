// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package nullable

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// service answers with the adoption it was sent.
type service struct{}

func (service) Adopt(_ context.Context, opts *AdoptServiceRequestOptions) (*AdoptResponseData, error) {
	return NewAdoptResponseData(opts.Body), nil
}

// newSession serves the service over HTTP, registers the tools on an MCP server that calls it
// through the client, and connects an MCP client in process.
func newSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	server := mcp.NewServer(&mcp.Implementation{Name: "adoptions", Version: "1"}, nil)
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

// TestListTools checks that a nullable allOf of a $ref and a nullable enum are anyOf with null.
func TestListTools(t *testing.T) {
	t.Parallel()

	session := newSession(t)

	res, err := session.ListTools(t.Context(), nil)

	require.NoError(t, err)
	require.Len(t, res.Tools, 1)
	defs := res.Tools[0].InputSchema.(map[string]any)["$defs"].(map[string]any)
	assert.Equal(t, map[string]any{
		"family": map[string]any{"type": "string"},
		"pet": map[string]any{
			"anyOf":       []any{map[string]any{"allOf": []any{map[string]any{"$ref": "#/$defs/Pet"}}}, map[string]any{"type": "null"}},
			"description": "The pet, or null while the family has not chosen one.",
		},
		"priority": map[string]any{
			"anyOf": []any{map[string]any{"type": "string", "enum": []any{"high", "low"}}, map[string]any{"type": "null"}},
		},
	}, defs["Adoption"].(map[string]any)["properties"])
}

func TestCallTool(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	session := newSession(t)
	tests := []struct {
		name    string
		body    map[string]any
		want    any
		isError bool
	}{
		{
			name: "A pet and a priority",
			body: map[string]any{"family": "Ito", "pet": map[string]any{"name": "Rex"}, "priority": "high"},
			want: map[string]any{"family": "Ito", "pet": map[string]any{"name": "Rex"}, "priority": "high"},
		},
		{
			name: "Null for both",
			body: map[string]any{"family": "Ito", "pet": nil, "priority": nil},
			want: map[string]any{"family": "Ito"},
		},
		{name: "A pet without its name", body: map[string]any{"family": "Ito", "pet": map[string]any{}}, isError: true},
		{name: "A priority outside the enum", body: map[string]any{"family": "Ito", "priority": "soon"}, isError: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "adopt", Arguments: map[string]any{"body": tc.body}})

			require.NoError(t, err)
			assert.Equal(t, tc.isError, res.IsError)
			assert.Equal(t, tc.want, res.StructuredContent)
		})
	}
}
