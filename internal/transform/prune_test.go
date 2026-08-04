// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package transform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/internal/oasdoc"
)

func TestPrune(t *testing.T) {
	t.Parallel()

	runGolden(t, "prune", Prune)
}

func TestPruneEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		src         string
		want        string
		wantChanged bool
	}{
		{name: "No components", src: "paths: {}\n", want: "paths: {}\n"},
		{
			name:        "Ref to a missing component and no paths",
			src:         "webhooks:\n  a: {post: {requestBody: {$ref: '#/components/requestBodies/Missing'}}}\ncomponents:\n  schemas:\n    A: {type: string}\n",
			want:        "webhooks:\n  a: {post: {requestBody: {$ref: '#/components/requestBodies/Missing'}}}\n",
			wantChanged: true,
		},
		{name: "Components that is not a map", src: "components: []\n", want: "components: []\n"},
		{
			name:        "Nothing used removes components",
			src:         "paths: {}\ncomponents:\n  schemas:\n    A: {type: string}\n",
			want:        "paths: {}\n",
			wantChanged: true,
		},
		{
			name: "Everything used keeps components",
			src:  "paths:\n  /a:\n    get:\n      parameters: [{$ref: '#/components/parameters/P'}]\ncomponents:\n  parameters:\n    P: {name: p, in: query}\n",
			want: "paths:\n  /a:\n    get:\n      parameters: [{$ref: '#/components/parameters/P'}]\ncomponents:\n  parameters:\n    P: {name: p, in: query}\n",
		},
		{
			name:        "Refs that are not component pointers",
			src:         "paths:\n  /a:\n    get:\n      parameters: [{$ref: '#/components/parameters'}, {$ref: '#/components/%zz/x'}, {$ref: '#/paths/~1b'}]\ncomponents:\n  parameters:\n    P: {name: p, in: query}\n",
			want:        "paths:\n  /a:\n    get:\n      parameters: [{$ref: '#/components/parameters'}, {$ref: '#/components/%zz/x'}, {$ref: '#/paths/~1b'}]\n",
			wantChanged: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc, err := oasdoc.Parse([]byte(tt.src), "spec.yaml")
			require.NoError(t, err)

			isChanged, _ := Prune(doc)
			out, err := doc.Marshal()
			require.NoError(t, err)
			assert.Equal(t, tt.wantChanged, isChanged)
			assert.Equal(t, tt.want, string(out))
		})
	}
}
