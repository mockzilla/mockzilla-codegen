// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package files

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// service answers with the bytes it got, as text, their media type and the name of a file.
type service struct{}

func (service) SetPhoto(_ context.Context, opts *SetPhotoServiceRequestOptions) (*SetPhotoResponseData, error) {
	content, err := opts.Body.Bytes()
	if err != nil {
		return nil, err
	}
	return NewSetPhotoResponseData(&Received{Text: string(content), ContentType: opts.Body.ContentType()}), nil
}

func (service) AddNote(_ context.Context, opts *AddNoteServiceRequestOptions) (*AddNoteResponseData, error) {
	content, err := opts.Body.Attachment.Bytes()
	if err != nil {
		return nil, err
	}
	return NewAddNoteResponseData(&Received{Title: &opts.Body.Title, Name: new(opts.Body.Attachment.Name()), Text: string(content), ContentType: opts.Body.Attachment.ContentType()}), nil
}

// newSession serves the service over HTTP, registers the tools on an MCP server that calls it
// through the client, and connects an MCP client in process.
func newSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	server := mcp.NewServer(&mcp.Implementation{Name: "files", Version: "1"}, nil)
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

// TestListTools checks that a binary body and a binary field of a form say they come as base64,
// and the body its media type.
func TestListTools(t *testing.T) {
	t.Parallel()

	session := newSession(t)

	res, err := session.ListTools(t.Context(), nil)

	require.NoError(t, err)
	require.Len(t, res.Tools, 2)
	assert.Equal(t, map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"body"},
		"properties":           map[string]any{"body": map[string]any{"$ref": "#/$defs/Note"}},
		"$defs": map[string]any{"Note": map[string]any{
			"type":     "object",
			"required": []any{"title", "attachment"},
			"properties": map[string]any{
				"title":      map[string]any{"type": "string"},
				"attachment": map[string]any{"type": "string", "format": "binary", "contentEncoding": "base64"},
			},
		}},
	}, res.Tools[0].InputSchema)
	assert.Equal(t, map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"id", "body"},
		"properties": map[string]any{
			"id":   map[string]any{"type": "string"},
			"body": map[string]any{"type": "string", "format": "binary", "contentEncoding": "base64", "contentMediaType": "image/png"},
		},
	}, res.Tools[1].InputSchema)
}

// TestCallTool checks that the base64 of a call reaches the API as the bytes it encodes.
func TestCallTool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tool string
		args map[string]any
		want map[string]any
	}{
		{
			name: "A photo",
			tool: "set_photo",
			args: map[string]any{"id": "1", "body": "aGVsbG8="},
			want: map[string]any{"text": "hello", "contentType": "image/png"},
		},
		{
			name: "An attachment of a form",
			tool: "add_note",
			args: map[string]any{"body": map[string]any{"title": "greeting", "attachment": "aGVsbG8="}},
			want: map[string]any{"title": "greeting", "name": "blob", "text": "hello", "contentType": "application/octet-stream"},
		},
	}

	session := newSession(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: tc.tool, Arguments: tc.args})

			require.NoError(t, err)
			assert.False(t, res.IsError)
			assert.Equal(t, tc.want, res.StructuredContent)
		})
	}
}

// TestCallToolWithoutBase64 checks that text that is no base64 is turned away before it reaches
// the API.
func TestCallToolWithoutBase64(t *testing.T) {
	t.Parallel()

	session := newSession(t)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "set_photo", Arguments: map[string]any{"id": "1", "body": "hello"}})

	require.NoError(t, err)
	assert.True(t, res.IsError)
	require.Len(t, res.Content, 1)
	assert.Contains(t, res.Content[0].(*mcp.TextContent).Text, "illegal base64 data at input byte 4")
}
