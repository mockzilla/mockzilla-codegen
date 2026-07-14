// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

// Package provider is the boundary between the generator and the library that reads OpenAPI.
package provider

import (
	"context"

	"github.com/mockzilla/codegen/internal/diag"
	"github.com/mockzilla/codegen/internal/spec"
)

// Provider reads specs. Implementations never print; problems come back as diagnostics or errors.
type Provider interface {
	ApplyOverlay(ctx context.Context, data, overlay []byte) ([]byte, []diag.Diagnostic, error)
	Bundle(ctx context.Context, src Source) (*Bundled, error)
	Parse(ctx context.Context, data []byte, opts ParseOptions) (*spec.Document, []diag.Diagnostic, error)
}

// Source is a root spec. Relative file refs resolve against BaseDir; remote refs need AllowRemote.
type Source struct {
	Data        []byte
	Path        string
	BaseDir     string
	AllowRemote bool
}

// Bundled is a spec with external refs lifted into components; Origins says where each came from.
type Bundled struct {
	Data    []byte
	Origins map[string]diag.Origin
}

// ParseOptions.Positions, taken before any transform, win over positions in the parsed bytes.
type ParseOptions struct {
	File      string
	Positions map[string]diag.Origin
}
