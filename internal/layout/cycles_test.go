// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package layout

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanImportCycles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     string
		parts   []Part
		wantMsg string
	}{
		{
			name: "Uses in one direction across folders are fine",
			cfg:  "output:\n  file: ./api/gen.go\n  files: {./models/types.go: [models.types]}\n",
			parts: []Part{
				{ID: "models.types"},
				{ID: "models.params", Uses: []PartID{"models.types", "models.types"}},
			},
		},
		{
			name: "Two folders that use each other form a cycle",
			cfg:  "output:\n  file: ./api/gen.go\n  files: {./models/types.go: [models.types]}\n",
			parts: []Part{
				{ID: "models.types", Uses: []PartID{"models.unions"}},
				{ID: "models.unions", Uses: []PartID{"models.types"}},
			},
			wantMsg: "import cycle: api -> models -> api (models.unions uses models.types, models.types uses models.unions)",
		},
		{
			name: "Cycle through three folders",
			cfg: "output:\n  file: ./a/gen.go\n  files:\n" +
				"    ./b/gen.go: [models.enums]\n    ./c/gen.go: [models.unions]\n",
			parts: []Part{
				{ID: "models.types", Uses: []PartID{"models.enums"}},
				{ID: "models.enums", Uses: []PartID{"models.unions"}},
				{ID: "models.unions", Uses: []PartID{"models.types"}},
			},
			wantMsg: "import cycle: a -> b -> c -> a " +
				"(models.types uses models.enums, models.enums uses models.unions, models.unions uses models.types)",
		},
		{
			name: "Folders reached twice without a cycle are fine",
			cfg: "output:\n  file: ./a/gen.go\n  files:\n" +
				"    ./b/gen.go: [models.enums]\n    ./c/gen.go: [models.unions]\n    ./d/gen.go: [models.params]\n",
			parts: []Part{
				{ID: "models.types", Uses: []PartID{"models.enums", "models.unions"}},
				{ID: "models.enums", Uses: []PartID{"models.params"}},
				{ID: "models.unions", Uses: []PartID{"models.params"}},
				{ID: "models.params"},
			},
		},
		{
			name:  "Uses of unknown parts and of the own folder are ignored",
			cfg:   "output: {file: ./gen.go}\n",
			parts: []Part{{ID: "models.types", Uses: []PartID{"models.types", "server.router"}}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			l, err := Plan(parseConfig(t, tc.cfg, "/work"), tc.parts, workModule)

			if tc.wantMsg == "" {
				require.NoError(t, err)
				assert.NotNil(t, l)
				return
			}
			require.ErrorIs(t, err, ErrImportCycle)
			assert.EqualError(t, err, tc.wantMsg)
		})
	}
}
