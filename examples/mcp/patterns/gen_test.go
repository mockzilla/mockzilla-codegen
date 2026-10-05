// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package patterns

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// service answers with the tag it was given.
type service struct{}

func (service) AddTag(_ context.Context, opts *AddTagServiceRequestOptions) (*AddTagResponseData, error) {
	return NewAddTagResponseData(&Added{Name: opts.Body.Name, Owner: opts.Body.Owner, Color: opts.Query.Color}), nil
}

// newSession serves the service over HTTP, registers the tools on an MCP server that calls it
// through the client, and connects an MCP client in process. Registering panics on a pattern the
// SDK cannot compile.
func newSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	server := mcp.NewServer(&mcp.Implementation{Name: "tags", Version: "1"}, nil)
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

// TestListTools checks which patterns and defaults the input schema keeps, and in what form.
func TestListTools(t *testing.T) {
	t.Parallel()

	noSpace := "^[^\\t-\\r \\xA0\U00001680\U00002000-\U0000200A\U00002028\U00002029\U0000202F\U0000205F\U00003000\U0000FEFF]+$"
	session := newSession(t)

	res, err := session.ListTools(t.Context(), nil)

	require.NoError(t, err)
	require.Len(t, res.Tools, 1)
	assert.Equal(t, map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"body"},
		"properties": map[string]any{
			"color": map[string]any{"type": "string", "pattern": "^[0-9a-f]{6}$", "description": "The color as six hex digits."},
			"body":  map[string]any{"$ref": "#/$defs/Tag"},
		},
		"$defs": map[string]any{"Tag": map[string]any{
			"type":     "object",
			"required": []any{"name"},
			"properties": map[string]any{
				"name":  map[string]any{"type": "string", "pattern": `^[\x21-\x7E]+$`, "description": "Printable characters, no space."},
				"label": map[string]any{"type": "string", "pattern": noSpace, "description": "One word, with no space of any kind."},
				"owner": map[string]any{"type": "string", "description": "Anyone but root."},
			},
		}},
	}, res.Tools[0].InputSchema)
}

// TestCallTool checks calls that fit the patterns the input schema keeps.
func TestCallTool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args map[string]any
		want map[string]any
	}{
		{name: "A name that matches", args: map[string]any{"body": map[string]any{"name": "go"}}, want: map[string]any{"name": "go"}},
		{name: "A color that matches", args: map[string]any{"color": "ff8800", "body": map[string]any{"name": "go"}}, want: map[string]any{"name": "go", "color": "ff8800"}},
		{name: "The owner the left out pattern turns away", args: map[string]any{"body": map[string]any{"name": "go", "owner": "root"}}, want: map[string]any{"name": "go", "owner": "root"}},
	}

	session := newSession(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "add_tag", Arguments: tc.args})

			require.NoError(t, err)
			assert.False(t, res.IsError)
			assert.Equal(t, tc.want, res.StructuredContent)
		})
	}
}

// TestCallToolOutsideAPattern checks that the SDK turns a call away before it reaches the API.
func TestCallToolOutsideAPattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{name: "A name with a space", args: map[string]any{"body": map[string]any{"name": "go lang"}}, want: `validating /$defs/Tag/properties/name: pattern: "go lang" does not match regular expression`},
		{name: "A label with a no-break space", args: map[string]any{"body": map[string]any{"name": "go", "label": "go\U000000A0lang"}}, want: `validating /$defs/Tag/properties/label: pattern: "go`},
		{name: "A color by name", args: map[string]any{"color": "red", "body": map[string]any{"name": "go"}}, want: `validating /properties/color: pattern: "red" does not match regular expression`},
	}

	session := newSession(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "add_tag", Arguments: tc.args})

			require.NoError(t, err)
			assert.True(t, res.IsError)
			require.Len(t, res.Content, 1)
			assert.Contains(t, res.Content[0].(*mcp.TextContent).Text, tc.want)
		})
	}
}
