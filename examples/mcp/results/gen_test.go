// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package results

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

// files are what GetPhoto stores per pet, each in its own format.
var files = map[string]runtime.File{
	"Rex":  runtime.NewFile([]byte("png"), "", "image/png"),
	"Bark": runtime.NewFile([]byte("wav"), "", "audio/wav"),
	"Tom":  runtime.NewFile([]byte("pdf"), "", "application/pdf"),
}

// service knows one pet, the files of three and the icon of the store.
type service struct{}

func (service) CountPets(context.Context, *CountPetsServiceRequestOptions) (*CountPetsResponseData, error) {
	return NewCountPetsResponseData(new(2)), nil
}

func (service) FindPet(_ context.Context, opts *FindPetServiceRequestOptions) (*FindPetResponseData, error) {
	if opts.Query.Name != "Rex" {
		return NewFindPetResponseData204(), nil
	}
	return NewFindPetResponseData200(&Pet{Name: "Rex"}), nil
}

func (service) GetPhoto(_ context.Context, opts *GetPhotoServiceRequestOptions) (*GetPhotoResponseData, error) {
	f := files[opts.Query.Name]
	return NewGetPhotoResponseData(&f), nil
}

func (service) GetIcon(context.Context, *GetIconServiceRequestOptions) (*GetIconResponseData, error) {
	return NewGetIconResponseData([]byte("icon")), nil
}

// newSession serves the service over HTTP, registers the tools on an MCP server that calls it
// through the client, and connects an MCP client in process.
func newSession(t *testing.T) *mcp.ClientSession {
	t.Helper()

	srv := httptest.NewServer(NewRouter(service{}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL)
	require.NoError(t, err)

	server := mcp.NewServer(&mcp.Implementation{Name: "results", Version: "1"}, nil)
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

// TestCallTools checks that structured content is always a JSON object: a pet as it is, a number
// under result. A 204 answers ok.
func TestCallTools(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tool string
		args map[string]any
		want any
		text string
	}{
		{name: "A number is wrapped", tool: "count_pets", want: map[string]any{"result": float64(2)}, text: `{"result":2}`},
		{name: "An object is as it is", tool: "find_pet", args: map[string]any{"name": "Rex"}, want: map[string]any{"name": "Rex"}, text: `{"name":"Rex"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			session := newSession(t)

			res, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: tc.tool, Arguments: tc.args})

			require.NoError(t, err)
			assert.False(t, res.IsError)
			assert.Equal(t, tc.want, res.StructuredContent)
			assert.Equal(t, []mcp.Content{&mcp.TextContent{Text: tc.text}}, res.Content)
		})
	}
}
