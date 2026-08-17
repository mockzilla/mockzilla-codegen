// Copyright (c) 2026 Mockzilla
// SPDX-License-Identifier: MIT
// Licensed under the MIT License, see LICENSE in the repository root. This copyright notice and
// permission notice shall be included in all copies or substantial portions of the Software.

package nested

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mockzilla/mockzilla-codegen/pkg/runtime"
)

func TestOwnerValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		owner Owner
		want  string
	}{
		{name: "Valid owner", owner: Owner{Pets: []Pet{{Name: "Rex"}}}},
		{name: "Required slice", want: "pets: is required"},
		{name: "Item of a slice", owner: Owner{Pets: []Pet{{Name: "Rex"}, {}}}, want: "pets[1].name: must be at least 1 characters long"},
		{name: "Pointer field", owner: Owner{Pets: []Pet{}, Best: &Pet{}}, want: "best.name: must be at least 1 characters long"},
		{name: "Map values in key order", owner: Owner{Pets: []Pet{}, ByName: map[string]Pet{"b": {}, "a": {}}}, want: `byName["a"].name: must be at least 1 characters long; byName["b"].name: must be at least 1 characters long`},
		{name: "Slice of slices", owner: Owner{Pets: []Pet{}, Grid: [][]int{{1}, {0, -1}}}, want: "grid[1][1]: must be at least 0"},
		{name: "Union variant", owner: Owner{Pets: []Pet{}, Contact: &OwnerContact{String: new("555")}}, want: "contact: must match ^\\+[0-9]+$"},
		{name: "Union with two variants", owner: Owner{Pets: []Pet{}, Contact: &OwnerContact{String: new("+1"), Email: new(runtime.Email("a@b.c"))}}, want: "contact: exactly one variant must be set, found 2"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := tc.owner.Validate()

			if tc.want == "" {
				require.NoError(t, err)
				return
			}
			require.EqualError(t, err, tc.want)
		})
	}
}

func TestPetsValidate(t *testing.T) {
	t.Parallel()

	require.EqualError(t, Pets{{Name: "a"}, {}, {Name: "c"}}.Validate(), "must have at most 2 items; [1].name: must be at least 1 characters long")
}
