// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package bundle

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJoin(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		base    string
		ref     string
		want    string
		wantErr bool
	}{
		{name: "Relative file", base: "specs", ref: "./schemas/Pet.yaml", want: "specs/schemas/Pet.yaml"},
		{name: "Parent folder", base: "specs/paths", ref: "../Pet.yaml", want: "specs/Pet.yaml"},
		{name: "Percent-encoded file name", base: "specs", ref: "my%20pet.yaml", want: "specs/my pet.yaml"},
		{name: "Absolute file", base: "specs", ref: "/shared/Pet.yaml", want: "/shared/Pet.yaml"},
		{name: "URL ref ignores the base", base: "specs", ref: "https://example.com/Pet.yaml", want: "https://example.com/Pet.yaml"},
		{name: "Relative to a URL", base: "https://example.com/specs/openapi.yaml", ref: "../common/Pet.yaml", want: "https://example.com/common/Pet.yaml"},
		{name: "Bad ref under a URL", base: "https://example.com/openapi.yaml", ref: "%zz", wantErr: true},
		{name: "Bad URL base", base: "https://example.com/%zz", ref: "Pet.yaml", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := join(tt.base, tt.ref)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, filepath.FromSlash(tt.want), filepath.FromSlash(got))
		})
	}
}

func TestStem(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"specs/schemas/Pet.yaml":                    "Pet",
		"https://example.com/specs/common.json?v=1": "common",
		"https://example.com/%zz/odd.yaml":          "odd",
	}
	for loc, want := range tests {
		assert.Equal(t, want, stem(loc), loc)
	}
}

func TestFolders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		loc  string
		base string
		want []string
	}{
		{name: "Folders below the root spec, nearest first", loc: "api/x/b/Pet.yaml", base: "api", want: []string{"b", "x"}},
		{name: "Folders above the root spec are left out", loc: "api/Pet.yaml", base: "api"},
		{name: "Parent steps are skipped", loc: "shared/Pet.yaml", base: "api", want: []string{"shared"}},
		{name: "A file the root folder cannot reach", loc: "/abs/Pet.yaml", base: "api"},
		{name: "URL path folders", loc: "https://example.com/v1/specs/Pet.yaml", base: "api", want: []string{"specs", "v1"}},
		{name: "Bad URL", loc: "https://example.com/%zz/Pet.yaml", base: "api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, folders(filepath.FromSlash(tt.loc), filepath.FromSlash(tt.base)))
		})
	}
}

func TestTitle(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"common":    "Common",
		"pet-types": "PetTypes",
		"v1.2":      "V12",
		"_":         "",
		"été":       "Été",
	}
	for in, want := range tests {
		assert.Equal(t, want, title(in), in)
	}
}
