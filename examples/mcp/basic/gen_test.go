// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package basic

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errNoPet = errors.New("no such pet")

// service keeps pets in memory.
type service struct {
	pets map[int]Pet
}

func (s *service) ListPets(_ context.Context, opts *ListPetsServiceRequestOptions) (*ListPetsResponseData, error) {
	pets := ListPetsResponse200{}
	for _, id := range slices.Sorted(maps.Keys(s.pets)) {
		pets = append(pets, s.pets[id])
	}
	if opts.Query.Limit != nil && *opts.Query.Limit < len(pets) {
		pets = pets[:*opts.Query.Limit]
	}
	return NewListPetsResponseData(pets), nil
}

func (s *service) CreatePet(_ context.Context, opts *CreatePetServiceRequestOptions) (*CreatePetResponseData, error) {
	if _, ok := s.pets[opts.Body.ID]; ok {
		return NewCreatePetResponseData409(new(NewProblem("a pet with that id exists"))), nil
	}
	s.pets[opts.Body.ID] = *opts.Body
	return NewCreatePetResponseData201(opts.Body), nil
}

func (s *service) GetPet(_ context.Context, opts *GetPetServiceRequestOptions) (*GetPetResponseData, error) {
	p, ok := s.pets[opts.PathParams.ID]
	if !ok {
		return NewGetPetResponseData404(new(NewProblem("no such pet"))), nil
	}
	return NewGetPetResponseData200(&p), nil
}

func (s *service) DeletePet(_ context.Context, opts *DeletePetServiceRequestOptions) (*DeletePetResponseData, error) {
	if _, ok := s.pets[opts.PathParams.ID]; !ok {
		return nil, errNoPet
	}
	delete(s.pets, opts.PathParams.ID)
	return NewDeletePetResponseData(), nil
}

func (*service) Ping(context.Context, *PingServiceRequestOptions) (*PingResponseData, error) {
	return NewPingResponseData(new("pong")), nil
}

// newSession serves the service over HTTP, registers the tools on an MCP server that calls it
// through the client, and connects an MCP client in process.
func newSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	srv := httptest.NewServer(NewRouter(&service{pets: map[int]Pet{}}))
	t.Cleanup(srv.Close)
	c, err := NewPetClient(srv.URL)
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

func TestListTools(t *testing.T) {
	t.Parallel()

	session := newSession(t)

	res, err := session.ListTools(t.Context(), nil)

	require.NoError(t, err)
	tools := make(map[string]*mcp.Tool, len(res.Tools))
	for _, tool := range res.Tools {
		tools[tool.Name] = tool
	}
	assert.Equal(t, []string{"create_pet", "delete_pet", "get_pet", "list_pets", "ping"}, slices.Sorted(maps.Keys(tools)))
	assert.Equal(t, "List the pets\nReturns every pet, the newest first.", tools["list_pets"].Description)
	assert.Equal(t, &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true}, tools["list_pets"].Annotations)
	assert.Equal(t, &mcp.ToolAnnotations{IdempotentHint: true}, tools["delete_pet"].Annotations)
	assert.Equal(t, &mcp.ToolAnnotations{}, tools["create_pet"].Annotations)
	assert.Equal(t, map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"limit": map[string]any{"type": "integer", "minimum": float64(1), "maximum": float64(100), "description": "How many pets to return at most."},
		},
	}, tools["list_pets"].InputSchema)
	assert.Equal(t, map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"body"},
		"properties": map[string]any{
			"body": map[string]any{"$ref": "#/$defs/Pet", "description": "The pet to add."},
		},
		"$defs": map[string]any{
			"Pet": map[string]any{
				"type":     "object",
				"required": []any{"id", "name"},
				"properties": map[string]any{
					"id":   map[string]any{"type": "integer", "description": "A number that is unique among pets."},
					"name": map[string]any{"type": "string", "minLength": float64(1)},
					"kind": map[string]any{"type": "string", "enum": []any{"dog", "cat", "bird"}},
				},
			},
		},
	}, tools["create_pet"].InputSchema)
	assert.Equal(t, map[string]any{"type": "object", "additionalProperties": false}, tools["ping"].InputSchema)
}

func TestCallTools(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	session := newSession(t)
	call := func(name string, args map[string]any) *mcp.CallToolResult {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		require.NoError(t, err)
		return res
	}
	structured := func(res *mcp.CallToolResult) string {
		data, err := json.Marshal(res.StructuredContent)
		require.NoError(t, err)
		return string(data)
	}

	created := call("create_pet", map[string]any{"body": map[string]any{"id": 1, "name": "Rex", "kind": "dog"}})
	assert.False(t, created.IsError)
	assert.JSONEq(t, `{"id":1,"name":"Rex","kind":"dog"}`, structured(created))
	assert.Equal(t, `{"id":1,"name":"Rex","kind":"dog"}`, created.Content[0].(*mcp.TextContent).Text, "the JSON is the text content too")

	call("create_pet", map[string]any{"body": map[string]any{"id": 2, "name": "Tom"}})
	listed := call("list_pets", map[string]any{"limit": 1})
	assert.False(t, listed.IsError)
	assert.JSONEq(t, `{"result":[{"id":1,"name":"Rex","kind":"dog"}]}`, structured(listed), "a list goes under result")

	all := call("list_pets", nil)
	assert.JSONEq(t, `{"result":[{"id":1,"name":"Rex","kind":"dog"},{"id":2,"name":"Tom"}]}`, structured(all))

	deleted := call("delete_pet", map[string]any{"id": 2})
	assert.False(t, deleted.IsError)
	assert.Equal(t, []mcp.Content{&mcp.TextContent{Text: "ok"}}, deleted.Content)

	pong := call("ping", nil)
	assert.Equal(t, []mcp.Content{&mcp.TextContent{Text: "pong"}}, pong.Content)
	assert.Nil(t, pong.StructuredContent)
}

func TestIntegersAbove2To53KeepEveryDigit(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	session := newSession(t)
	call := func(name string, args map[string]any) *mcp.CallToolResult {
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		require.NoError(t, err)
		return res
	}

	created := call("create_pet", map[string]any{"body": map[string]any{"id": 9007199254740993, "name": "Rex"}})
	require.False(t, created.IsError)
	assert.Equal(t, `{"id":9007199254740993,"name":"Rex"}`, created.Content[0].(*mcp.TextContent).Text)

	found := call("get_pet", map[string]any{"id": 9007199254740993})
	assert.False(t, found.IsError)
	rounded := call("get_pet", map[string]any{"id": 9007199254740992})
	assert.True(t, rounded.IsError, "the id a float64 rounds to is another pet")
}

func TestToolErrors(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	session := newSession(t)
	tests := []struct {
		name     string
		tool     string
		args     map[string]any
		wantText string
	}{
		{
			name:     "A documented error status carries the error type's message and the body",
			tool:     "get_pet",
			args:     map[string]any{"id": 7},
			wantText: "unexpected status 404 Not Found: no such pet\n{\"detail\":\"no such pet\"}",
		},
		{
			name:     "An error status the spec does not document carries the body",
			tool:     "delete_pet",
			args:     map[string]any{"id": 7},
			wantText: "unexpected status 500 Internal Server Error\n{\"error\":\"internal server error\"}",
		},
		{
			name:     "Input that fails the schema never reaches the API",
			tool:     "list_pets",
			args:     map[string]any{"limit": 0},
			wantText: "validating /properties/limit: minimum",
		},
		{
			name:     "A parameter the schema does not know is rejected",
			tool:     "list_pets",
			args:     map[string]any{"limt": 5},
			wantText: `unexpected additional properties ["limt"]`,
		},
		{
			name:     "A required body left out is rejected",
			tool:     "create_pet",
			wantText: `required: missing properties: ["body"]`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tc.tool, Arguments: tc.args})

			require.NoError(t, err)
			assert.True(t, res.IsError)
			require.Len(t, res.Content, 1)
			assert.Contains(t, res.Content[0].(*mcp.TextContent).Text, tc.wantText)
		})
	}
}
