// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package selected

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// service echoes what it is asked.
type service struct{}

func (service) GetItem(_ context.Context, opts *GetItemServiceRequestOptions) (*GetItemResponseData, error) {
	item := &Item{ID: opts.PathParams.ID, Tenant: opts.Headers.XTenant}
	if opts.Query != nil {
		item.Compared = opts.Query.ID
	}
	return NewGetItemResponseData(item), nil
}

func (service) PutItem(_ context.Context, opts *PutItemServiceRequestOptions) (*PutItemResponseData, error) {
	return NewPutItemResponseData(opts.Body), nil
}

func (service) DeleteItem(context.Context, *DeleteItemServiceRequestOptions) (*DeleteItemResponseData, error) {
	return NewDeleteItemResponseData(), nil
}

func (service) Reset(context.Context, *ResetServiceRequestOptions) (*ResetResponseData, error) {
	return NewResetResponseData(), nil
}

// newSession serves the service over HTTP, registers the tools on an MCP server that calls it
// through the client, and connects an MCP client in process.
func newSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	server := mcp.NewServer(&mcp.Implementation{Name: "items", Version: "1"}, nil)
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

// TestListTools checks that default-skip leaves every operation out unless x-mcp turns it on,
// and that x-mcp names and describes the tool.
func TestListTools(t *testing.T) {
	t.Parallel()

	session := newSession(t)

	res, err := session.ListTools(t.Context(), nil)

	require.NoError(t, err)
	require.Len(t, res.Tools, 2)
	assert.Equal(t, "delete_item", res.Tools[0].Name)
	assert.Equal(t, "Remove an item", res.Tools[0].Description)
	assert.Equal(t, "fetch_item", res.Tools[1].Name)
	assert.Equal(t, "Fetch one item by its number. Ask for the tenant when it is not known.", res.Tools[1].Description)
	assert.Equal(t, map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"id", "X-Tenant"},
		"properties": map[string]any{
			"id":       map[string]any{"type": "integer", "description": "The number of the item."},
			"query_id": map[string]any{"type": "integer", "description": "The number of the item to compare with."},
			"X-Tenant": map[string]any{"type": "string"},
		},
	}, res.Tools[1].InputSchema, "the query parameter that shares its name with the path parameter gets its location in front")
}

func TestCallTools(t *testing.T) {
	t.Parallel()

	session := newSession(t)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "fetch_item", Arguments: map[string]any{"id": 7, "query_id": 8, "X-Tenant": "acme"}})

	require.NoError(t, err)
	assert.False(t, res.IsError)
	data, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":7,"tenant":"acme","compared":8}`, string(data))
}

// TestInput shows the input type of a tool, with the field of the query parameter renamed like
// its property.
func TestInput(t *testing.T) {
	t.Parallel()

	in := GetItemToolInput{ID: 7, QueryID: new(8), XTenant: "acme"}

	data, err := json.Marshal(in)

	require.NoError(t, err)
	assert.JSONEq(t, `{"id":7,"query_id":8,"X-Tenant":"acme"}`, string(data))
}
