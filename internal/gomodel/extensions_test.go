// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package gomodel

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mockzilla/mockzilla-codegen/internal/extension"
	"github.com/mockzilla/mockzilla-codegen/pkg/config"
)

func TestExtensionsInModel(t *testing.T) {
	t.Parallel()

	opts := testOptions()
	opts.ExtraTags = []string{"yaml"}

	checkGolden(t, "extensions", "extensions", opts)
}

func TestGoType(t *testing.T) {
	t.Parallel()

	listed := []config.Import{{Package: "github.com/google/uuid"}, {Package: "example.com/shop/tenant", Alias: "tn"}, {Package: "embed", Alias: "_"}}
	tests := []struct {
		name    string
		typ     extension.Type
		imports []config.Import
		want    Type
	}{
		{name: "Builtin", typ: extension.Type{Name: "int64"}, want: Builtin{Name: "int64"}},
		{name: "Bytes", typ: extension.Type{Name: "[]byte"}, want: Slice{Elem: byteType}},
		{name: "Standard library type", typ: extension.Type{Name: "time.Duration"}, want: Qualified{Import: Import{Path: "time"}, Name: "Duration"}},
		{
			name: "Type of an imported package",
			typ:  extension.Type{Name: "dec.Decimal", Path: "github.com/shopspring/decimal", Alias: "dec"},
			want: Qualified{Import: Import{Path: "github.com/shopspring/decimal", Alias: "dec"}, Name: "Decimal"},
		},
		{
			name:    "Type of a package the config imports",
			typ:     extension.Type{Name: "uuid.UUID"},
			imports: listed,
			want:    Qualified{Import: Import{Path: "github.com/google/uuid", Alias: "uuid"}, Name: "UUID"},
		},
		{
			name:    "Type of a package the config imports under an alias",
			typ:     extension.Type{Name: "tn.ID"},
			imports: listed,
			want:    Qualified{Import: Import{Path: "example.com/shop/tenant", Alias: "tn"}, Name: "ID"},
		},
		{
			name:    "Import of the config wins over the standard library package of its name",
			typ:     extension.Type{Name: "time.Duration"},
			imports: []config.Import{{Package: "example.com/shop/time"}},
			want:    Qualified{Import: Import{Path: "example.com/shop/time", Alias: "time"}, Name: "Duration"},
		},
		{
			name:    "x-go-type-import wins over the import of the config",
			typ:     extension.Type{Name: "uuid.UUID", Path: "github.com/gofrs/uuid"},
			imports: listed,
			want:    Qualified{Import: Import{Path: "github.com/gofrs/uuid"}, Name: "UUID"},
		},
		{
			name:    "Import under _ names no package",
			typ:     extension.Type{Name: "embed.FS"},
			imports: listed,
			want:    Qualified{Import: Import{Path: "embed"}, Name: "FS"},
		},
		{name: "Composite type as written", typ: extension.Type{Name: "[]uuid.UUID"}, imports: listed, want: Builtin{Name: "[]uuid.UUID"}},
		{name: "Two dots as written", typ: extension.Type{Name: "a.b.C"}, want: Builtin{Name: "a.b.C"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tc.want, goType(&tc.typ, tc.imports))
		})
	}
}
