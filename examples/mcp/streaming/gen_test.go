// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package streaming

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// service answers every prompt whole, as JSON.
type service struct{}

func (service) Chat(_ context.Context, opts *ChatServiceRequestOptions) (*ChatResponseData, error) {
	return NewChatResponseData(&Reply{Text: "You said: " + opts.Body.Text}), nil
}

func (service) ListEvents(context.Context, *ListEventsServiceRequestOptions) (*ListEventsResponseData, error) {
	return NewListEventsResponseData("data: {\"seq\":1}\n\n"), nil
}

// newSession serves the service over HTTP, registers the tools on an MCP server that calls it
// through the client, and connects an MCP client in process.
func newSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	server := mcp.NewServer(&mcp.Implementation{Name: "chat", Version: "1"}, nil)
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

// TestChat calls an operation that documents a JSON response next to a streamed one: the tool
// reads it as JSON.
func TestChat(t *testing.T) {
	t.Parallel()

	session := newSession(t)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "chat", Arguments: map[string]any{"body": map[string]any{"text": "hi"}}})

	require.NoError(t, err)
	assert.False(t, res.IsError)
	data, err := json.Marshal(res.StructuredContent)
	require.NoError(t, err)
	assert.JSONEq(t, `{"text":"You said: hi"}`, string(data))
}

// TestListEvents calls an operation that answers with a stream only, which a tool result cannot
// carry: the tool is an error result that says so.
func TestListEvents(t *testing.T) {
	t.Parallel()

	session := newSession(t)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "list_events"})

	require.NoError(t, err)
	assert.True(t, res.IsError)
	assert.Equal(t, []mcp.Content{&mcp.TextContent{Text: ErrMCPStreaming.Error()}}, res.Content)
}
