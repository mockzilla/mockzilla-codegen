// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package tools_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/examples/layout/by-family/client"
	"github.com/mockzilla/mockzilla-codegen/examples/layout/by-family/models"
	"github.com/mockzilla/mockzilla-codegen/examples/layout/by-family/server"
	"github.com/mockzilla/mockzilla-codegen/examples/layout/by-family/tools"
)

// shop knows order 1 alone.
type shop struct{}

func (shop) CreateOrder(_ context.Context, opts *server.CreateOrderServiceRequestOptions) (*server.CreateOrderResponseData, error) {
	return server.NewCreateOrderResponseData(&models.Order{ID: "2", Status: models.StatusOpen, Items: opts.Body.Items}), nil
}

func (shop) GetOrder(_ context.Context, opts *server.GetOrderServiceRequestOptions) (*server.GetOrderResponseData, error) {
	if opts.PathParams.ID != "1" {
		return server.NewGetOrderResponseData404(&models.Problem{Detail: "no such order"}), nil
	}
	return server.NewGetOrderResponseData200(&models.Order{ID: "1", Status: models.StatusPaid}), nil
}

// TestAcrossFamilies calls the tools of one package, which call the client of another, which
// calls the server of a third, all on the models of a fourth.
func TestAcrossFamilies(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(server.NewRouter(shop{}))
	t.Cleanup(srv.Close)
	c, err := client.NewClient(srv.URL)
	require.NoError(t, err)
	s := mcp.NewServer(&mcp.Implementation{Name: "orders", Version: "1"}, nil)
	tools.NewMCPTools(c).Register(s)

	ctx := t.Context()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := s.Connect(ctx, serverTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = serverSession.Wait() })
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	call := func(name string, args map[string]any) *mcp.CallToolResult {
		res, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		require.NoError(t, callErr)
		return res
	}

	got := call("get_order", map[string]any{"id": "1", "expand": "items"})
	assert.False(t, got.IsError)
	data, err := json.Marshal(got.StructuredContent)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"1","status":"paid"}`, string(data))

	missing := call("get_order", map[string]any{"id": "9"})
	assert.True(t, missing.IsError)

	created := call("create_order", map[string]any{"body": map[string]any{"items": []any{map[string]any{"sku": "tea", "quantity": 2}}}})
	assert.False(t, created.IsError)
	assert.JSONEq(t, `{"id":"2","status":"open","items":[{"sku":"tea","quantity":2}]}`, created.Content[0].(*mcp.TextContent).Text)
}
