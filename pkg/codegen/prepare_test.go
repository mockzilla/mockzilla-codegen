// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package codegen

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/codegen/pkg/config"
)

const splitOut = `openapi: 3.1.0
info: {title: split, version: '1'}
paths:
  /pets:
    get:
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema: {$ref: '#/components/schemas/pet'}
components:
  schemas:
    pet:
      type: object
      properties:
        name: {type: string}
`

func TestPrepare(t *testing.T) {
	t.Parallel()

	dir := filepath.Join("testdata", "split")
	data, err := os.ReadFile(filepath.Join(dir, "openapi.yaml"))
	require.NoError(t, err)

	tests := []struct {
		name string
		cfg  string
		opts []Option
	}{
		{name: "Spec from spec.path", cfg: "spec: {path: openapi.yaml}\n"},
		{name: "Spec in memory", cfg: "{}", opts: []Option{WithSpec(data)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := config.Parse([]byte(tt.cfg), dir)
			require.NoError(t, err)

			got, diags, err := Prepare(context.Background(), cfg, tt.opts...)
			require.NoError(t, err)
			assert.Equal(t, splitOut, string(got))
			assert.Empty(t, diags)
		})
	}
}

func TestPrepareDiagnostics(t *testing.T) {
	t.Parallel()

	cfg, err := config.Parse([]byte("spec:\n  path: openapi.yaml\n  filter:\n    include: {tags: [none]}\n"), filepath.Join("testdata", "split"))
	require.NoError(t, err)

	_, diags, err := Prepare(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, []Diagnostic{{
		Severity: SeverityWarning,
		Code:     "filter-empty",
		File:     filepath.Join("testdata", "split", "openapi.yaml"),
		Line:     1,
		Col:      1,
		Message:  "the filter removed every operation",
	}}, diags)
	assert.Equal(t, "warning", diags[0].Severity.String())
}

func TestPrepareError(t *testing.T) {
	t.Parallel()

	cfg, err := config.Parse([]byte("{}"), "")
	require.NoError(t, err)

	_, _, err = Prepare(context.Background(), cfg)
	require.Error(t, err)
}
