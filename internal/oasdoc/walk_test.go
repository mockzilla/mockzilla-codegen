// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package oasdoc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v4"

	"github.com/mockzilla/codegen/internal/diag"
)

const walkDoc = `paths:
  /a:
    get:
      responses:
        '200': {$ref: '#/components/responses/Ok'}
components:
  schemas:
    Pet:
      properties:
        $ref: {type: string}
        owner: {$ref: 'models.yaml#/Owner'}
      example: &ex {name: x}
    Copy: *ex
`

func TestWalk(t *testing.T) {
	t.Parallel()

	d, err := Parse([]byte(walkDoc), "spec.yaml")
	require.NoError(t, err)

	var got []string
	d.Walk(func(ptr string, _ *yaml.Node) bool {
		got = append(got, ptr)
		return ptr != "/paths/~1a/get"
	})
	assert.Equal(t, []string{
		"",
		"/paths",
		"/paths/~1a",
		"/paths/~1a/get",
		"/components",
		"/components/schemas",
		"/components/schemas/Pet",
		"/components/schemas/Pet/properties",
		"/components/schemas/Pet/properties/$ref",
		"/components/schemas/Pet/properties/$ref/type",
		"/components/schemas/Pet/properties/owner",
		"/components/schemas/Pet/properties/owner/$ref",
		"/components/schemas/Pet/example",
		"/components/schemas/Pet/example/name",
		"/components/schemas/Copy",
	}, got)
}

func TestRefs(t *testing.T) {
	t.Parallel()

	d, err := Parse([]byte(walkDoc), "spec.yaml")
	require.NoError(t, err)

	assert.Equal(t, []Ref{
		{Owner: "/paths/~1a/get/responses/200", Value: "#/components/responses/Ok"},
		{Owner: "/components/schemas/Pet/properties/owner", Value: "models.yaml#/Owner"},
	}, d.Refs())
}

func TestPositions(t *testing.T) {
	t.Parallel()

	d, err := Parse([]byte("openapi: 3.1.0\npaths:\n  /a:\n    get: {}\ntags:\n  - name: x\n"), "spec.yaml")
	require.NoError(t, err)

	assert.Equal(t, map[string]diag.Origin{
		"":               {File: "spec.yaml", Line: 1, Col: 1},
		"/openapi":       {File: "spec.yaml", Line: 1, Col: 1},
		"/paths":         {File: "spec.yaml", Line: 2, Col: 1},
		"/paths/~1a":     {File: "spec.yaml", Line: 3, Col: 3},
		"/paths/~1a/get": {File: "spec.yaml", Line: 4, Col: 5},
		"/tags":          {File: "spec.yaml", Line: 5, Col: 1},
		"/tags/0":        {File: "spec.yaml", Line: 6, Col: 5},
		"/tags/0/name":   {File: "spec.yaml", Line: 6, Col: 5},
	}, d.Positions())
}
