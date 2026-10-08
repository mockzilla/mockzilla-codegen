// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package bundle

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/diag"
	"github.com/mockzilla/mockzilla-codegen/internal/oasdoc"
)

func TestRunGolden(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		dir  string
	}{
		{name: "Paths, schemas, cycles, siblings and a JSON file spread over folders", dir: "split"},
		{name: "Clashing names get a file or folder suffix, then a number", dir: "clash"},
		{name: "Root components that only ref another file keep their names", dir: "homes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := filepath.Join("testdata", tt.dir)
			root := filepath.Join(dir, "openapi.yaml")
			doc := parseFile(t, root)

			isChanged, diags, err := Run(context.Background(), Input{Doc: doc, Location: root, Load: readFile})
			require.NoError(t, err)
			assert.True(t, isChanged)
			golden(t, filepath.Join(dir, "out.golden"), render(t, doc, dir, diags))
		})
	}
}

func TestRunRemote(t *testing.T) {
	t.Parallel()

	files := map[string]string{
		"https://example.com/specs/schemas/Pet.yaml": "type: object\nproperties:\n  err: {$ref: 'https://other.example.com/common.yaml#/Error'}\n",
		"https://other.example.com/common.yaml":      "Error: {type: string}\n",
	}
	root := "openapi: 3.1.0\npaths:\n  /a:\n    get:\n      responses:\n        '200':\n          content:\n            application/json:\n              schema: {$ref: ./schemas/Pet.yaml}\n"
	doc, err := oasdoc.Parse([]byte(root), "https://example.com/specs/openapi.yaml")
	require.NoError(t, err)

	isChanged, diags, err := Run(context.Background(), Input{Doc: doc, Location: "https://example.com/specs/openapi.yaml", Load: mapLoader(files)})
	require.NoError(t, err)
	out, err := doc.Marshal()
	require.NoError(t, err)
	assert.True(t, isChanged)
	assert.Empty(t, diags)
	assert.Equal(t, `openapi: 3.1.0
paths:
  /a:
    get:
      responses:
        '200':
          content:
            application/json:
              schema: {$ref: '#/components/schemas/Pet'}
components:
  schemas:
    Pet:
      type: object
      properties:
        err: {$ref: '#/components/schemas/Error'}
    Error: {type: string}
`, string(out))
	assert.Equal(t, diag.Origin{File: "https://other.example.com/common.yaml", Line: 1, Col: 8}, doc.Positions()["/components/schemas/Error"])
}

func TestRunInMemory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input Input
		want  string
	}{
		{
			name:  "Base dir resolves refs of bytes with no location",
			input: Input{BaseDir: "specs"},
			want:  "specs/pet.yaml",
		},
		{
			name:  "Refs of bytes with nothing else resolve in the working folder",
			input: Input{},
			want:  "pet.yaml",
		},
		{
			name:  "Base dir wins over the location's folder",
			input: Input{Location: "api/openapi.yaml", BaseDir: "shared"},
			want:  "shared/pet.yaml",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc, err := oasdoc.Parse([]byte("openapi: 3.1.0\ncomponents:\n  schemas:\n    A: {$ref: ./pet.yaml}\n"), "")
			require.NoError(t, err)
			var got string
			tt.input.Doc = doc
			tt.input.Load = func(_ context.Context, loc string) ([]byte, error) {
				got = loc
				return []byte("type: string\n"), nil
			}

			_, _, err = Run(context.Background(), tt.input)
			require.NoError(t, err)
			assert.Equal(t, filepath.FromSlash(tt.want), got)
		})
	}
}

func TestRunReadsSlashEscapesInJSON(t *testing.T) {
	t.Parallel()

	doc, err := oasdoc.Parse([]byte("openapi: 3.1.0\ncomponents:\n  schemas:\n    A: {$ref: ./a.json}\n"), "spec.yaml")
	require.NoError(t, err)

	_, _, err = Run(context.Background(), Input{Doc: doc, Location: "spec.yaml", Load: mapLoader(map[string]string{"a.json": `{"pattern": "^a\/b$"}`})})
	require.NoError(t, err)
	out, err := doc.Marshal()
	require.NoError(t, err)
	assert.Contains(t, string(out), "pattern: ^a/b$")
}

func TestRunWithLocalRefsOnly(t *testing.T) {
	t.Parallel()

	src := "openapi: 3.1.0\ncomponents:\n  schemas:\n    A: {$ref: '#/components/schemas/B'}\n    B: {type: string}\n"
	doc, err := oasdoc.Parse([]byte(src), "spec.yaml")
	require.NoError(t, err)

	isChanged, diags, err := Run(context.Background(), Input{Doc: doc, Location: "spec.yaml"})
	require.NoError(t, err)
	assert.False(t, isChanged)
	assert.Nil(t, diags)
}

func TestRunErrors(t *testing.T) {
	t.Parallel()

	const head = "openapi: 3.1.0\npaths:\n"
	schemaRef := func(ref string) string {
		return head + "  /a:\n    get:\n      responses:\n        '200':\n          description: ok\n          content:\n            application/json:\n              schema: {$ref: '" + ref + "'}\n"
	}
	files := map[string]string{
		"pet.yaml":    "type: string\nname: pet\n",
		"a.yaml":      "$ref: ./b.yaml\n",
		"b.yaml":      "$ref: ./a.yaml\n",
		"loop.yaml":   "get:\n  callbacks:\n    cb:\n      '{$url}': {$ref: ./loop.yaml}\n",
		"broken.yaml": "a: [b\n",
		"list.yaml":   "- a\n",
		"shape.yaml":  "type: object\ndiscriminator:\n  propertyName: kind\n  mapping: {a: ./missing.yaml}\n",
	}

	tests := []struct {
		name    string
		src     string
		loc     string
		wantErr error
		wantAt  string
	}{
		{name: "Missing file", src: schemaRef("./missing.yaml"), wantErr: ErrLoad, wantAt: "spec.yaml:10:30"},
		{name: "Missing pointer", src: schemaRef("./pet.yaml#/Nope"), wantErr: ErrTarget, wantAt: "spec.yaml:10:30"},
		{name: "Fragment that is not a pointer", src: schemaRef("./pet.yaml#Pet"), wantErr: ErrFragment},
		{name: "Refs that only point at each other", src: schemaRef("./a.yaml"), wantErr: ErrCycle},
		{name: "Path item that inlines itself", src: head + "  /a: {$ref: ./loop.yaml}\n", wantErr: ErrCycle, wantAt: "loop.yaml:4:24"},
		{name: "Inlined target that is not an object", src: head + "  /a: {$ref: './pet.yaml#/name'}\n", wantErr: ErrInline},
		{name: "File that is not YAML", src: schemaRef("./broken.yaml"), wantErr: ErrLoad},
		{name: "File that is not an object", src: schemaRef("./list.yaml"), wantErr: ErrLoad},
		{name: "Discriminator target missing", src: schemaRef("./shape.yaml"), wantErr: ErrLoad, wantAt: "shape.yaml:4:16"},
		{
			name:    "Root component with a bad escape in a ref under a URL",
			src:     "openapi: 3.1.0\ncomponents:\n  schemas:\n    A: {$ref: './%zz.yaml'}\n",
			loc:     "https://example.com/openapi.yaml",
			wantErr: ErrLoad,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			loc := cmp.Or(tt.loc, "spec.yaml")
			doc, err := oasdoc.Parse([]byte(tt.src), loc)
			require.NoError(t, err)

			_, _, err = Run(context.Background(), Input{Doc: doc, Location: loc, Load: mapLoader(files)})
			require.ErrorIs(t, err, tt.wantErr)
			assert.ErrorContains(t, err, tt.wantAt)
		})
	}
}

func readFile(_ context.Context, loc string) ([]byte, error) {
	return os.ReadFile(loc)
}

func parseFile(t *testing.T, path string) *oasdoc.Doc {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	doc, err := oasdoc.Parse(data, path)
	require.NoError(t, err)
	return doc
}

// render prints the bundled spec, then each pointer where content switches to another file.
func render(t *testing.T, doc *oasdoc.Doc, dir string, diags []diag.Diagnostic) string {
	t.Helper()

	out, err := doc.Marshal()
	require.NoError(t, err)

	var b strings.Builder
	b.Write(out)
	b.WriteString("# grafted\n")
	positions := doc.Positions()
	for _, ptr := range slices.Sorted(maps.Keys(positions)) {
		at := positions[ptr]
		parent := ptr[:max(strings.LastIndex(ptr, "/"), 0)]
		if ptr == "" || positions[parent].File == at.File {
			continue
		}
		fmt.Fprintf(&b, "%s %s:%d:%d\n", ptr, rel(dir, at.File), at.Line, at.Col)
	}
	b.WriteString("# diagnostics\n")
	for _, d := range diags {
		fmt.Fprintf(&b, "%s %s %s %s:%d:%d %s\n", d.Severity, d.Code, d.Pointer, rel(dir, d.Origin.File), d.Origin.Line, d.Origin.Col,
			strings.ReplaceAll(d.Message, dir+string(filepath.Separator), ""))
	}
	return b.String()
}

func rel(dir, file string) string {
	r, err := filepath.Rel(dir, file)
	if err != nil {
		return file
	}
	return filepath.ToSlash(r)
}

// golden compares got with name; UPDATE=1 rewrites the file instead.
func golden(t *testing.T, name, got string) {
	t.Helper()

	if os.Getenv("UPDATE") != "" {
		require.NoError(t, os.WriteFile(name, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(name)
	require.NoError(t, err)
	assert.Equal(t, string(want), got)
}

func mapLoader(files map[string]string) Loader {
	return func(_ context.Context, loc string) ([]byte, error) {
		data, ok := files[filepath.ToSlash(loc)]
		if !ok {
			return nil, os.ErrNotExist
		}
		return []byte(data), nil
	}
}
