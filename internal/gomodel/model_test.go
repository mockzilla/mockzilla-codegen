// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/naming"
	"github.com/mockzilla/mockzilla-codegen/internal/provider"
	"github.com/mockzilla/mockzilla-codegen/internal/provider/libopenapi"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

func testOptions() Options {
	return Options{IntType: "int", Descriptions: true, EnumPrefix: true, Namer: naming.New(nil)}
}

// checkGolden builds testdata/<fixture>.yaml and compares the model dump and Build's diagnostics
// with testdata/<name>.golden. UPDATE=1 rewrites the golden file instead.
func checkGolden(t *testing.T, fixture, name string, opts Options) {
	t.Helper()

	src, err := os.ReadFile(filepath.Join("testdata", fixture+".yaml"))
	require.NoError(t, err)
	doc, _, err := libopenapi.New().Parse(t.Context(), src, provider.ParseOptions{File: fixture + ".yaml"})
	require.NoError(t, err)

	m, diags := Build(doc, opts)
	var b strings.Builder
	b.WriteString(Dump(m))
	b.WriteString("---\n")
	for _, d := range diags {
		fmt.Fprintf(&b, "%s %s %d:%d %s: %s\n", d.Severity, d.Code, d.Origin.Line, d.Origin.Col, d.Pointer, d.Message)
	}

	path := filepath.Join("testdata", name+".golden")
	if os.Getenv("UPDATE") != "" {
		require.NoError(t, os.WriteFile(path, []byte(b.String()), 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, string(want), b.String())
}

func TestOptionsFrom(t *testing.T) {
	t.Parallel()

	n := naming.New([]string{"PSP"})
	tests := []struct {
		name string
		cfg  *config.Config
		want Options
	}{
		{
			name: "Nothing set gives the defaults",
			cfg:  &config.Config{Naming: config.Naming{Initialisms: []string{"PSP"}}},
			want: Options{IntType: "int", Descriptions: true, EnumPrefix: true, Namer: n, IsValidated: true},
		},
		{
			name: "Models and naming settings",
			cfg: &config.Config{
				Naming: config.Naming{Initialisms: []string{"PSP"}, EnumPrefix: new(false)},
				Models: &config.Models{
					IntType:      "int64",
					Descriptions: new(false),
					ExtraTags:    []string{"yaml"},
					Validation:   config.ModelValidation{Response: true},
					ErrorMapping: map[string]string{"Problem": "detail", "Error": "message"},
				},
			},
			want: Options{
				IntType:          "int64",
				ExtraTags:        []string{"yaml"},
				Namer:            n,
				IsValidated:      true,
				ValidateResponse: true,
				ErrorMapping:     map[string]string{"Problem": "detail", "Error": "message"},
				Reserved:         []string{"NewError", "NewProblem"},
			},
		},
		{
			name: "Validation skipped",
			cfg: &config.Config{
				Naming: config.Naming{Initialisms: []string{"PSP"}},
				Models: &config.Models{Validation: config.ModelValidation{Skip: true, Response: true}},
			},
			want: Options{IntType: "int", Descriptions: true, EnumPrefix: true, Namer: n},
		},
		{
			name: "Server and client names are reserved",
			cfg: &config.Config{
				Naming: config.Naming{Initialisms: []string{"PSP"}},
				Server: &config.Server{Name: "PetService", Validation: config.ServerValidation{Response: true}, Scaffold: config.Scaffold{Service: "service.go", Middleware: "middleware.go"}},
				Client: &config.Client{},
			},
			want: Options{
				IntType:            "int",
				Descriptions:       true,
				EnumPrefix:         true,
				Namer:              n,
				IsValidated:        true,
				ValidateResponse:   true,
				IsServer:           true,
				HasResponseHeaders: true,
				Reserved: slices.Concat(
					[]string{"PetServiceInterface"}, serverNames, []string{"PetService", "NewPetService", "ErrNotImplemented"}, middlewareNames,
					[]string{"Client", "NewClient", "ClientOption", "ClientInterface", "HTTPDoer", "RequestEditor", "WithHTTPClient", "WithTimeout", "WithRequestEditor"},
				),
				Methods:           Methods{Client: []string{"Request"}},
				OperationSuffixes: []string{"ServiceRequestOptions", "ResponseData", "RequestOptions"},
			},
		},
		{
			name: "A client with envelopes reserves its response names and declares response headers",
			cfg: &config.Config{
				Naming: config.Naming{Initialisms: []string{"PSP"}},
				Client: &config.Client{Name: "PetClient", WithResponse: true},
			},
			want: Options{
				IntType:            "int",
				Descriptions:       true,
				EnumPrefix:         true,
				Namer:              n,
				IsValidated:        true,
				HasResponseHeaders: true,
				Reserved:           []string{"PetClient", "NewPetClient", "PetClientOption", "PetClientInterface", "HTTPDoer", "RequestEditor", "WithHTTPClient", "WithTimeout", "WithRequestEditor"},
				Methods:            Methods{Client: []string{"Request", "WithResponse"}},
				OperationSuffixes:  []string{"RequestOptions", "Response"},
			},
		},
		{
			name: "A streaming client holds the names of its stream methods",
			cfg: &config.Config{
				Naming: config.Naming{Initialisms: []string{"PSP"}},
				Client: &config.Client{Streaming: true},
			},
			want: Options{
				IntType:           "int",
				Descriptions:      true,
				EnumPrefix:        true,
				Namer:             n,
				IsValidated:       true,
				Reserved:          []string{"Client", "NewClient", "ClientOption", "ClientInterface", "HTTPDoer", "RequestEditor", "WithHTTPClient", "WithTimeout", "WithRequestEditor"},
				Methods:           Methods{Client: []string{"Request"}, Stream: []string{"Stream"}},
				OperationSuffixes: []string{"RequestOptions"},
			},
		},
		{
			name: "A streaming client with envelopes holds the names of both stream methods",
			cfg: &config.Config{
				Naming: config.Naming{Initialisms: []string{"PSP"}},
				Client: &config.Client{WithResponse: true, Streaming: true},
			},
			want: Options{
				IntType:            "int",
				Descriptions:       true,
				EnumPrefix:         true,
				Namer:              n,
				IsValidated:        true,
				HasResponseHeaders: true,
				Reserved:           []string{"Client", "NewClient", "ClientOption", "ClientInterface", "HTTPDoer", "RequestEditor", "WithHTTPClient", "WithTimeout", "WithRequestEditor"},
				Methods:            Methods{Client: []string{"Request", "WithResponse"}, Stream: []string{"Stream", "StreamWithResponse"}},
				OperationSuffixes:  []string{"RequestOptions", "Response"},
			},
		},
		{
			name: "MCP reserves its names, the Register method, the tool input suffix and the Tool methods",
			cfg: &config.Config{
				Naming: config.Naming{Initialisms: []string{"PSP"}},
				Client: &config.Client{},
				MCP:    &config.MCP{DefaultSkip: true},
			},
			want: Options{
				IntType:            "int",
				Descriptions:       true,
				EnumPrefix:         true,
				Namer:              n,
				IsValidated:        true,
				Reserved:           []string{"Client", "NewClient", "ClientOption", "ClientInterface", "HTTPDoer", "RequestEditor", "WithHTTPClient", "WithTimeout", "WithRequestEditor", "MCPTools", "NewMCPTools", "ErrMCPStreaming"},
				ReservedOperations: []string{"Register"},
				Methods:            Methods{Client: []string{"Request"}, Tool: []string{"Tool"}, IsToolSkipped: true},
				OperationSuffixes:  []string{"RequestOptions", "ToolInput"},
			},
		},
		{
			name: "Imports are passed on",
			cfg: &config.Config{
				Naming:  config.Naming{Initialisms: []string{"PSP"}},
				Imports: []config.Import{{Package: "github.com/google/uuid"}, {Package: "embed", Alias: "_"}},
			},
			want: Options{
				IntType:      "int",
				Descriptions: true,
				EnumPrefix:   true,
				Namer:        n,
				IsValidated:  true,
				Imports:      []config.Import{{Package: "github.com/google/uuid"}, {Package: "embed", Alias: "_"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, OptionsFrom(tt.cfg))
		})
	}
}
